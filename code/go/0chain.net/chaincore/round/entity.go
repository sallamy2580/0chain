package round

import (
	"0chain.net/chaincore/node"
	"0chain.net/core/ememorystore"
	. "0chain.net/core/logging"
	"context"
	"fmt"
	"math/rand"
	"os"
	"runtime/pprof"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"0chain.net/chaincore/block"
	"0chain.net/core/datastore"
	"go.uber.org/zap"
)

const (
	RoundShareVRF = iota
	RoundVRFComplete
	RoundGenerating
	RoundGenerated
	RoundCollectingBlockProposals
	RoundStateVerificationTimedOut
	RoundStateFinalizing
	RoundStateFinalized
)

/*Round - data structure for the round */
type Round struct {
	datastore.NOIDField
	Number        int64 `json:"number"`
	RandomSeed    int64 `json:"round_random_seed"`
	hasRandomSeed uint32

	// For generator, this is the block the miner is generating till a notraization is received
	// For a verifier, this is the block that is currently the best block received for verification.
	// Once a round is finalized, this is the finalized block of the given round
	Block     *block.Block `json:"-"`
	BlockHash string       `json:"block_hash"`
	VRFOutput string       `json:"vrf_output"` //TODO: VRFOutput == rbooutput?

	minerPerm       []int
	state           int32
	proposedBlocks  []*block.Block
	notarizedBlocks []*block.Block
	Mutex           sync.RWMutex
	shares          map[string]*VRFShare

	timeoutCount     int32
	softTimeoutCount int32
	vrfStartTime     atomic.Value

	timeoutVotes map[int]int
	votersVoted  map[string]bool
	votesMutex   sync.Mutex
}

// RoundFactory - a factory to create a new round object specific to miner/sharder
type RoundFactory interface {
	CreateRoundF(roundNum int64) interface{}
}

//NewRound - Create a new round object
func NewRound(round int64) *Round {
	r := datastore.GetEntityMetadata("round").Instance().(*Round)
	r.Number = round
	return r
}

var roundEntityMetadata *datastore.EntityMetadataImpl

/*GetEntityMetadata - implementing the interface */
func (r *Round) GetEntityMetadata() datastore.EntityMetadata {
	return roundEntityMetadata
}

/*GetKey - returns the round number as the key */
func (r *Round) GetKey() datastore.Key {
	return datastore.ToKey(fmt.Sprintf("%v", r.GetRoundNumber()))
}

//GetRoundNumber - returns the round number
func (r *Round) GetRoundNumber() int64 {
	return r.Number
}

// GetTimeoutCount - returns the timeout count
func (r *Round) GetTimeoutCount() int {
	return r.getTimeoutCount()
}

func (r *Round) getTimeoutCount() int {
	return int(atomic.LoadInt32(&r.timeoutCount))
}

func (r *Round) setTimeoutCount(tc int) {
	atomic.StoreInt32(&r.timeoutCount, int32(tc))
}

// IncrementTimeoutCount - Increments timeout count
func (r *Round) IncrementTimeoutCount() {
	r.votesMutex.Lock()
	defer r.votesMutex.Unlock()
	var mostVotes int
	for k, v := range r.timeoutVotes {
		if v > mostVotes || (v == mostVotes && r.getTimeoutCount() > k) {
			mostVotes = v
			r.setTimeoutCount(k)
		}
	}
	r.timeoutVotes = make(map[int]int)
	r.votersVoted = make(map[string]bool)
	atomic.AddInt32(&r.timeoutCount, 1)
}

// SetTimeoutCount - sets the timeout count to given number if it is greater than existing and returns true. Else false.
func (r *Round) SetTimeoutCount(tc int) bool {
	if tc <= r.getTimeoutCount() {
		return false
	}
	r.setTimeoutCount(tc)
	return true
}

//SetRandomSeed - set the random seed of the round
func (r *Round) SetRandomSeedForNotarizedBlock(seed int64) {
	r.setRandomSeed(seed)
	//r.setState(RoundVRFComplete) RoundStateFinalizing??
	r.setHasRandomSeed(true)
	//r.Mutex.Lock()
	r.minerPerm = nil
	//r.Mutex.Unlock()
}

//SetRandomSeed - set the random seed of the round
func (r *Round) SetRandomSeed(seed int64) {
	if atomic.LoadUint32(&r.hasRandomSeed) == 1 {
		return
	}
	r.setRandomSeed(seed)
	r.setState(RoundVRFComplete)
	r.setHasRandomSeed(true)

	//r.Mutex.Lock()
	r.minerPerm = nil
	//r.Mutex.Unlock()
}

func (r *Round) setRandomSeed(seed int64) {
	atomic.StoreInt64(&r.RandomSeed, seed)
}

func (r *Round) setHasRandomSeed(b bool) {
	value := uint32(0)
	if b {
		value = 1
	}
	atomic.StoreUint32(&r.hasRandomSeed, value)
}

//GetRandomSeed - returns the random seed of the round
func (r *Round) GetRandomSeed() int64 {
	return atomic.LoadInt64(&r.RandomSeed)
}

// SetVRFOutput --sets the VRFOutput
func (r *Round) SetVRFOutput(rboutput string) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	r.VRFOutput = rboutput
}

// GetVRFOutput --gets the VRFOutput
func (r *Round) GetVRFOutput() string {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()
	return r.VRFOutput
}

/*AddNotarizedBlock - this will be concurrent as notarization is recognized by verifying as well as notarization message from others */
func (r *Round) AddNotarizedBlock(b *block.Block) (*block.Block, bool) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	b, _ = r.addProposedBlock(b)
	found := -1

	for i, blk := range r.notarizedBlocks {
		if blk.Hash == b.Hash {
			if blk != b {
				blk.MergeVerificationTickets(b.VerificationTickets)
			}
			return blk, false
		}
		if blk.RoundRank == b.RoundRank {
			found = i
		}
	}

	if found > -1 {
		fb := r.notarizedBlocks[found]
		Logger.Info("Removing the old notarized block with the same rank", zap.Int64("round", r.GetRoundNumber()), zap.String("hash", fb.Hash),
			zap.Int64("fb_RRS", fb.RoundRandomSeed), zap.Int("fb_toc", fb.RoundTimeoutCount), zap.Any("fb_Sender", fb.MinerID))
		//remove the old block with the same rank and add it below
		r.notarizedBlocks = append(r.notarizedBlocks[:found], r.notarizedBlocks[found+1:]...)
	}
	b.SetBlockNotarized()
	if r.Block == nil || r.Block.RoundRank > b.RoundRank {
		r.Block = b
	}
	rnb := append(r.notarizedBlocks, b)
	sort.Slice(rnb, func(i int, j int) bool { return rnb[i].ChainWeight > rnb[j].ChainWeight })
	r.notarizedBlocks = rnb
	return b, true
}

/*GetNotarizedBlocks - return all the notarized blocks associated with this round */
func (r *Round) GetNotarizedBlocks() []*block.Block {
	return r.notarizedBlocks
}

/*AddProposedBlock - this will be concurrent as notarization is recognized by verifying as well as notarization message from others */
func (r *Round) AddProposedBlock(b *block.Block) (*block.Block, bool) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	return r.addProposedBlock(b)
}

func (r *Round) addProposedBlock(b *block.Block) (*block.Block, bool) {
	for _, blk := range r.proposedBlocks {
		if blk.Hash == b.Hash {
			return blk, false
		}
	}
	r.proposedBlocks = append(r.proposedBlocks, b)
	sort.SliceStable(r.proposedBlocks, func(i, j int) bool { return r.proposedBlocks[i].RoundRank < r.proposedBlocks[j].RoundRank })
	return b, true
}

/*GetProposedBlocks - return all the blocks that have been proposed for this round */
func (r *Round) GetProposedBlocks() []*block.Block {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()
	return r.proposedBlocks
}

/*GetHeaviestNotarizedBlock - get the heaviest notarized block that we have in this round */
func (r *Round) GetHeaviestNotarizedBlock() *block.Block {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()
	rnb := r.notarizedBlocks
	if len(rnb) == 0 {
		return nil
	}
	return rnb[0]
}

/*GetBlocksByRank - return the currently stored blocks in the order of best rank for the round */
func (r *Round) GetBlocksByRank(blocks []*block.Block) []*block.Block {
	sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].RoundRank < blocks[j].RoundRank })
	return blocks
}

/*GetBestRankedNotarizedBlock - get the best ranked notarized block for this round */
func (r *Round) GetBestRankedNotarizedBlock() *block.Block {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()
	rnb := r.notarizedBlocks
	if len(rnb) == 0 {
		return nil
	}
	if len(rnb) == 1 {
		return rnb[0]
	}
	rnb = r.GetBlocksByRank(rnb)
	return rnb[0]
}

/*Finalize - finalize the round */
func (r *Round) Finalize(b *block.Block) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	r.setState(RoundStateFinalized)
	r.Block = b
	r.BlockHash = b.Hash
}

/*SetFinalizing - the round is being finalized */
func (r *Round) SetFinalizing() bool {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	if r.isFinalized() || r.isFinalizing() {
		return false
	}
	r.setState(RoundStateFinalizing)
	return true
}

/*IsFinalizing - is the round finalizing */
func (r *Round) IsFinalizing() bool {
	return r.isFinalizing()
}

func (r *Round) isFinalizing() bool {
	return r.getState() == RoundStateFinalizing
}

/*IsFinalized - indicates if the round is finalized */
func (r *Round) IsFinalized() bool {
	return r.isFinalized()
}

func (r *Round) isFinalized() bool {
	return r.getState() == RoundStateFinalized || r.GetRoundNumber() == 0
}

/*Provider - entity provider for client object */
func Provider() datastore.Entity {
	r := &Round{}
	r.initialize()
	r.timeoutVotes = make(map[int]int)
	r.votersVoted = make(map[string]bool)
	return r
}

func (r *Round) initialize() {
	r.notarizedBlocks = make([]*block.Block, 0, 1)
	r.proposedBlocks = make([]*block.Block, 0, 3)
	r.shares = make(map[string]*VRFShare)
	//when we restart a round we call this. So, explicitly, set them to default
	r.setHasRandomSeed(false)
	r.setRandomSeed(0)
}

/*Read - read round entity from store */
func (r *Round) Read(ctx context.Context, key datastore.Key) error {
	return r.GetEntityMetadata().GetStore().Read(ctx, key, r)
}

/*Write - write round entity to store */
func (r *Round) Write(ctx context.Context) error {
	return r.GetEntityMetadata().GetStore().Write(ctx, r)
}

/*Delete - delete round entity from store */
func (r *Round) Delete(ctx context.Context) error {
	return r.GetEntityMetadata().GetStore().Delete(ctx, r)
}

/*SetupEntity - setup the entity */
func SetupEntity(store datastore.Store) {
	roundEntityMetadata = datastore.MetadataProvider()
	roundEntityMetadata.Name = "round"
	roundEntityMetadata.DB = "roundsummarydb"
	roundEntityMetadata.Provider = Provider
	roundEntityMetadata.Store = store
	roundEntityMetadata.IDColumnName = "number"
	datastore.RegisterEntityMetadata("round", roundEntityMetadata)
}

//SetupRoundSummaryDB - setup the round summary db
func SetupRoundSummaryDB() {
	db, err := ememorystore.CreateDB("data/rocksdb/roundsummary")
	if err != nil {
		panic(err)
	}
	ememorystore.AddPool("roundsummarydb", db)
}

/*ComputeMinerRanks - Compute random order of n elements given the random seed of the round */
func (r *Round) ComputeMinerRanks(miners *node.Pool) {
	Logger.Info("compute miner ranks", zap.Any("num_miners", miners.Size()), zap.Any("round", r.Number))
	r.Mutex.Lock()
	r.minerPerm = rand.New(rand.NewSource(r.GetRandomSeed())).Perm(miners.Size())
	r.Mutex.Unlock()
}

/*GetMinerRank - get the rank of element at the elementIdx position based on the permutation of the round */
func (r *Round) GetMinerRank(miner *node.Node) int {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()
	if r.minerPerm == nil {
		pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
		Logger.DPanic(fmt.Sprintf("miner ranks not computed yet: %v", r.GetState()))
	}
	Logger.Info("get miner rank", zap.Any("minerPerm", r.minerPerm), zap.Any("miner", miner), zap.Any("round", r.Number))
	return r.minerPerm[miner.SetIndex]
}

/*GetMinersByRank - get the rnaks of the miners */
func (r *Round) GetMinersByRank(miners *node.Pool) []*node.Node {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()
	nodes := miners.Nodes
	rminers := make([]*node.Node, len(nodes))
	Logger.Info("get miners by rank", zap.Any("num_miners", len(nodes)), zap.Any("round", r.Number), zap.Any("r.minerPerm", r.minerPerm))
	for _, nd := range nodes {
		idx := r.minerPerm[nd.SetIndex]
		rminers[idx] = nd
	}
	return rminers
}

//Clear - implement interface
func (r *Round) Clear() {
}

//Restart - restart the round
func (r *Round) Restart() {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	r.initialize()
	r.Block = nil
	r.ResetState(RoundShareVRF)
	r.resetSoftTimeoutCount()

}

//AddAdditionalVRFShare - Adding additional VRFShare received for stats persp
func (r *Round) AddAdditionalVRFShare(share *VRFShare) bool {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	if _, ok := r.shares[share.party.GetKey()]; ok {
		Logger.Info("AddVRFShare Share is already there. Returning false.")
		return false
	}
	//r.setState(RoundShareVRF)
	r.shares[share.party.GetKey()] = share
	return true
}

//AddVRFShare - implement interface
func (r *Round) AddVRFShare(share *VRFShare, threshold int) bool {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	if len(r.getVRFShares()) >= threshold {
		//if we already have enough shares, do not add.
		Logger.Info("AddVRFShare Already at threshold. Returning false.")
		return false
	}
	if _, ok := r.shares[share.party.GetKey()]; ok {
		Logger.Info("AddVRFShare Share is already there. Returning false.")
		return false
	}
	r.setState(RoundShareVRF)
	r.shares[share.party.GetKey()] = share
	return true
}

//GetVRFShares - implement interface
func (r *Round) GetVRFShares() map[string]*VRFShare {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()
	return r.getVRFShares()
}

func (r *Round) getVRFShares() map[string]*VRFShare {
	result := make(map[string]*VRFShare, len(r.shares))
	for k, v := range r.shares {
		result[k] = v
	}
	return result
}

//GetState - get the state of the round
func (r *Round) GetState() int {
	return r.getState()
}

//SetState - set the state of the round in a progressive order
func (r *Round) SetState(state int) {
	r.setState(state)
}

//ResetState resets the state to any desired state
func (r *Round) ResetState(state int) {
	atomic.StoreInt32(&r.state, int32(state))
}

func (r *Round) getState() int {
	return int(atomic.LoadInt32(&r.state))
}

func (r *Round) setState(state int) {
	if state > r.getState() {
		atomic.StoreInt32(&r.state, int32(state))
	}
}

//HasRandomSeed - implement interface
func (r *Round) HasRandomSeed() bool {
	return atomic.LoadUint32(&r.hasRandomSeed) == 1
}

func (r *Round) AddTimeoutVote(num int, id string) {
	r.votesMutex.Lock()
	defer r.votesMutex.Unlock()
	if !r.votersVoted[id] {
		r.timeoutVotes[num]++
		r.votersVoted[id] = true
	}
}

func (r *Round) GetSoftTimeoutCount() int {
	return int(atomic.LoadInt32(&r.softTimeoutCount))
}

func (r *Round) IncSoftTimeoutCount() {
	atomic.AddInt32(&r.softTimeoutCount, 1)
}

func (r *Round) resetSoftTimeoutCount() {
	atomic.StoreInt32(&r.softTimeoutCount, 0)
}

func (r *Round) SetVrfStartTime(t time.Time) {
	r.vrfStartTime.Store(t)
}

func (r *Round) GetVrfStartTime() time.Time {
	value := r.vrfStartTime.Load()
	if value == nil {
		return time.Time{}
	}
	return value.(time.Time)
}
