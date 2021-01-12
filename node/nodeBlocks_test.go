package node_test

import (
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	apiBlock "github.com/ElrondNetwork/elrond-go/api/block"
	"github.com/ElrondNetwork/elrond-go/core"
	"github.com/ElrondNetwork/elrond-go/data"
	"github.com/ElrondNetwork/elrond-go/data/block"
	"github.com/ElrondNetwork/elrond-go/dataRetriever"
	mock2 "github.com/ElrondNetwork/elrond-go/factory/mock"
	"github.com/ElrondNetwork/elrond-go/node"
	"github.com/ElrondNetwork/elrond-go/node/mock"
	"github.com/ElrondNetwork/elrond-go/storage"
	"github.com/ElrondNetwork/elrond-go/testscommon"
	"github.com/ElrondNetwork/elrond-go/testscommon/bootstrapMocks"
	"github.com/ElrondNetwork/elrond-go/testscommon/economicsMocks"
	"github.com/ElrondNetwork/elrond-go/testscommon/mainFactoryMocks"
	"github.com/stretchr/testify/assert"
)

func TestGetBlockByHash_InvalidShardShouldErr(t *testing.T) {
	t.Parallel()

	n, _ := node.NewNode()

	blk, err := n.GetBlockByHash("invalidHash", false)
	assert.Error(t, err)
	assert.Nil(t, blk)
}

func TestGetBlockByHashFromHistoryNode(t *testing.T) {
	t.Parallel()

	historyProc := &testscommon.HistoryRepositoryStub{
		IsEnabledCalled: func() bool {
			return true
		},
		GetEpochByHashCalled: func(hash []byte) (uint32, error) {
			return 1, nil
		},
	}
	nonce := uint64(1)
	round := uint64(2)
	epoch := uint32(1)
	shardID := uint32(5)
	miniblockHeader := []byte("mbHash")
	headerHash := []byte("d08089f2ab739520598fd7aeed08c427460fe94f286383047f3f61951afc4e00")

	storerMock := mock.NewStorerMock()
	coreComponentsMock := getDefaultCoreComponents()
	processComponentsMock := getDefaultProcessComponents()
	processComponentsMock.HistoryRepositoryInternal = historyProc
	dataComponentsMock := getDefaultDataComponents()
	dataComponentsMock.Store = &mock.ChainStorerMock{
		GetStorerCalled: func(unitType dataRetriever.UnitType) storage.Storer {
			return storerMock
		},
	}

	n, _ := node.NewNode(
		node.WithCoreComponents(coreComponentsMock),
		node.WithProcessComponents(processComponentsMock),
		node.WithDataComponents(dataComponentsMock),
	)

	header := &block.Header{
		Nonce:   nonce,
		Round:   round,
		ShardID: shardID,
		Epoch:   epoch,
		MiniBlockHeaders: []block.MiniBlockHeader{
			{Hash: miniblockHeader},
		},
	}
	blockBytes, _ := json.Marshal(header)
	_ = storerMock.Put(headerHash, blockBytes)

	expectedBlock := &apiBlock.APIBlock{
		Nonce: nonce,
		Round: round,
		Shard: shardID,
		Epoch: epoch,
		Hash:  hex.EncodeToString(headerHash),
		MiniBlocks: []*apiBlock.APIMiniBlock{
			{
				Hash: hex.EncodeToString(miniblockHeader),
				Type: block.TxBlock.String(),
			},
		},
	}

	blk, err := n.GetBlockByHash(hex.EncodeToString(headerHash), false)
	assert.Nil(t, err)
	assert.Equal(t, expectedBlock, blk)
}

func TestGetBlockByHashFromNormalNode(t *testing.T) {
	t.Parallel()

	nonce := uint64(1)
	round := uint64(2)
	epoch := uint32(1)
	miniblockHeader := []byte("mbHash")
	headerHash := []byte("d08089f2ab739520598fd7aeed08c427460fe94f286383047f3f61951afc4e00")
	storerMock := mock.NewStorerMock()
	coreComponentsMock := getDefaultCoreComponents()
	processComponentsMock := getDefaultProcessComponents()
	processComponentsMock.ShardCoord = &mock.ShardCoordinatorMock{SelfShardId: core.MetachainShardId}
	processComponentsMock.HistoryRepositoryInternal = &testscommon.HistoryRepositoryStub{
		IsEnabledCalled: func() bool {
			return false
		},
	}
	dataComponentsMock := getDefaultDataComponents()
	dataComponentsMock.Store = &mock.ChainStorerMock{
		GetCalled: func(unitType dataRetriever.UnitType, key []byte) ([]byte, error) {
			return storerMock.Get(key)
		},
	}

	n, _ := node.NewNode(
		node.WithCoreComponents(coreComponentsMock),
		node.WithProcessComponents(processComponentsMock),
		node.WithDataComponents(dataComponentsMock),
	)

	header := &block.MetaBlock{
		Nonce: nonce,
		Round: round,
		Epoch: epoch,
		MiniBlockHeaders: []block.MiniBlockHeader{
			{Hash: miniblockHeader},
		},
	}
	headerBytes, _ := json.Marshal(header)
	_ = storerMock.Put(headerHash, headerBytes)

	expectedBlock := &apiBlock.APIBlock{
		Nonce: nonce,
		Round: round,
		Shard: core.MetachainShardId,
		Epoch: epoch,
		Hash:  hex.EncodeToString(headerHash),
		MiniBlocks: []*apiBlock.APIMiniBlock{
			{
				Hash: hex.EncodeToString(miniblockHeader),
				Type: block.TxBlock.String(),
			},
		},
		NotarizedBlocks: []*apiBlock.APINotarizedBlock{},
	}

	blk, err := n.GetBlockByHash(hex.EncodeToString(headerHash), false)
	assert.Nil(t, err)
	assert.Equal(t, expectedBlock, blk)
}

func TestGetBlockByNonceFromHistoryNode(t *testing.T) {
	t.Parallel()

	historyProc := &testscommon.HistoryRepositoryStub{
		IsEnabledCalled: func() bool {
			return true
		},
		GetEpochByHashCalled: func(hash []byte) (uint32, error) {
			return 1, nil
		},
	}

	nonce := uint64(1)
	round := uint64(2)
	epoch := uint32(1)
	shardID := uint32(5)
	miniblockHeader := []byte("mbHash")
	storerMock := mock.NewStorerMock()
	headerHash := "d08089f2ab739520598fd7aeed08c427460fe94f286383047f3f61951afc4e00"
	coreComponentsMock := getDefaultCoreComponents()
	processComponentsMock := getDefaultProcessComponents()
	processComponentsMock.HistoryRepositoryInternal = historyProc
	dataComponentsMock := getDefaultDataComponents()
	dataComponentsMock.Store = &mock.ChainStorerMock{
		GetCalled: func(unitType dataRetriever.UnitType, key []byte) ([]byte, error) {
			return hex.DecodeString(headerHash)
		},
		GetStorerCalled: func(unitType dataRetriever.UnitType) storage.Storer {
			return storerMock
		},
	}

	n, _ := node.NewNode(
		node.WithCoreComponents(coreComponentsMock),
		node.WithDataComponents(dataComponentsMock),
		node.WithProcessComponents(processComponentsMock),
	)

	header := &block.Header{
		Nonce:   nonce,
		Round:   round,
		ShardID: shardID,
		Epoch:   epoch,
		MiniBlockHeaders: []block.MiniBlockHeader{
			{Hash: miniblockHeader},
		},
	}
	headerBytes, _ := json.Marshal(header)
	_ = storerMock.Put(func() []byte { hashBytes, _ := hex.DecodeString(headerHash); return hashBytes }(), headerBytes)

	expectedBlock := &apiBlock.APIBlock{
		Nonce: nonce,
		Round: round,
		Shard: shardID,
		Epoch: epoch,
		Hash:  headerHash,
		MiniBlocks: []*apiBlock.APIMiniBlock{
			{
				Hash: hex.EncodeToString(miniblockHeader),
				Type: block.TxBlock.String(),
			},
		},
	}

	blk, err := n.GetBlockByNonce(1, false)
	assert.Nil(t, err)
	assert.Equal(t, expectedBlock, blk)
}

func TestGetBlockByNonceFromNormalNode(t *testing.T) {
	t.Parallel()

	nonce := uint64(1)
	round := uint64(2)
	epoch := uint32(1)
	shardID := uint32(5)
	miniblockHeader := []byte("mbHash")
	headerHash := "d08089f2ab739520598fd7aeed08c427460fe94f286383047f3f61951afc4e00"
	coreComponentsMock := getDefaultCoreComponents()
	processComponentsMock := getDefaultProcessComponents()
	dataComponentsMock := getDefaultDataComponents()
	dataComponentsMock.Store = &mock.ChainStorerMock{
		GetCalled: func(unitType dataRetriever.UnitType, key []byte) ([]byte, error) {
			if unitType == dataRetriever.ShardHdrNonceHashDataUnit {
				return hex.DecodeString(headerHash)
			}
			blk := &block.Header{
				Nonce:   nonce,
				Round:   round,
				ShardID: shardID,
				Epoch:   epoch,
				MiniBlockHeaders: []block.MiniBlockHeader{
					{Hash: miniblockHeader},
				},
			}
			blockBytes, _ := json.Marshal(blk)
			return blockBytes, nil
		},
	}

	processComponentsMock.HistoryRepositoryInternal = &testscommon.HistoryRepositoryStub{
		IsEnabledCalled: func() bool {
			return false
		},
	}

	n, _ := node.NewNode(
		node.WithCoreComponents(coreComponentsMock),
		node.WithDataComponents(dataComponentsMock),
		node.WithProcessComponents(processComponentsMock),
	)

	expectedBlock := &apiBlock.APIBlock{
		Nonce: nonce,
		Round: round,
		Shard: shardID,
		Epoch: epoch,
		Hash:  headerHash,
		MiniBlocks: []*apiBlock.APIMiniBlock{
			{
				Hash: hex.EncodeToString(miniblockHeader),
				Type: block.TxBlock.String(),
			},
		},
	}

	blk, err := n.GetBlockByNonce(1, false)
	assert.Nil(t, err)
	assert.Equal(t, expectedBlock, blk)
}

func getDefaultCoreComponents() *mock.CoreComponentsMock {
	return &mock.CoreComponentsMock{
		IntMarsh:            &testscommon.MarshalizerMock{},
		TxMarsh:             &testscommon.MarshalizerMock{},
		VmMarsh:             &testscommon.MarshalizerMock{},
		TxSignHasherField:   &testscommon.HasherStub{},
		Hash:                &testscommon.HasherStub{},
		UInt64ByteSliceConv: testscommon.NewNonceHashConverterMock(),
		AddrPubKeyConv:      testscommon.NewPubkeyConverterMock(32),
		ValPubKeyConv:       testscommon.NewPubkeyConverterMock(32),
		PathHdl:             &testscommon.PathManagerStub{},
		ChainIdCalled: func() string {
			return "chainID"
		},
		MinTransactionVersionCalled: func() uint32 {
			return 1
		},
		AppStatusHdl:        &testscommon.AppStatusHandlerStub{},
		WDTimer:             &testscommon.WatchdogMock{},
		Alarm:               &testscommon.AlarmSchedulerStub{},
		NtpTimer:            &testscommon.SyncTimerStub{},
		RoundHandlerField:   &testscommon.RoundHandlerMock{},
		EconomicsHandler:    &economicsMocks.EconomicsHandlerMock{},
		RatingsConfig:       &testscommon.RatingsInfoMock{},
		RatingHandler:       &testscommon.RaterMock{},
		NodesConfig:         &testscommon.NodesSetupStub{},
		StartTime:           time.Time{},
		EpochChangeNotifier: &mock.EpochNotifierStub{},
	}
}

func getDefaultProcessComponents() *mock2.ProcessComponentsMock {
	return &mock2.ProcessComponentsMock{
		NodesCoord: &mock.NodesCoordinatorMock{},
		ShardCoord: &testscommon.ShardsCoordinatorMock{
			NoShards:     1,
			CurrentShard: 0,
		},
		IntContainer:                   &mock.InterceptorsContainerStub{},
		ResFinder:                      &mock.ResolversFinderStub{},
		RoundHandlerField:              &testscommon.RoundHandlerMock{},
		EpochTrigger:                   &testscommon.EpochStartTriggerStub{},
		EpochNotifier:                  &mock.EpochStartNotifierStub{},
		ForkDetect:                     &mock.ForkDetectorMock{},
		BlockProcess:                   &mock.BlockProcessorStub{},
		BlackListHdl:                   &testscommon.TimeCacheStub{},
		BootSore:                       &mock.BootstrapStorerMock{},
		HeaderSigVerif:                 &mock.HeaderSigVerifierStub{},
		HeaderIntegrVerif:              &mock.HeaderIntegrityVerifierStub{},
		ValidatorStatistics:            &mock.ValidatorStatisticsProcessorMock{},
		ValidatorProvider:              &mock.ValidatorsProviderStub{},
		BlockTrack:                     &mock.BlockTrackerStub{},
		PendingMiniBlocksHdl:           &mock.PendingMiniBlocksHandlerStub{},
		ReqHandler:                     &mock.RequestHandlerStub{},
		TxLogsProcess:                  &mock.TxLogProcessorMock{},
		HeaderConstructValidator:       &mock.HeaderValidatorStub{},
		PeerMapper:                     &mock.NetworkShardingCollectorStub{},
		WhiteListHandlerInternal:       &mock.WhiteListHandlerStub{},
		WhiteListerVerifiedTxsInternal: &mock.WhiteListHandlerStub{},
	}
}

func getDefaultDataComponents() *mock.DataComponentsMock {
	return &mock.DataComponentsMock{
		BlockChain: &mock.ChainHandlerStub{},
		Store:      &mock.ChainStorerStub{},
		DataPool:   &testscommon.PoolsHolderMock{},
		MbProvider: &mock.MiniBlocksProviderStub{},
	}
}

func getDefaultBootstrapComponents() *mainFactoryMocks.BootstrapComponentsStub {
	return &mainFactoryMocks.BootstrapComponentsStub{
		Bootstrapper: &bootstrapMocks.EpochStartBootstrapperStub{
			TrieHolder:      &mock.TriesHolderStub{},
			StorageManagers: map[string]data.StorageManager{"0": &mock.StorageManagerStub{}},
			BootstrapCalled: nil,
		},
		BootstrapParams:      &bootstrapMocks.BootstrapParamsHandlerMock{},
		NodeRole:             "",
		ShCoordinator:        &mock.ShardCoordinatorMock{},
		HdrIntegrityVerifier: &mock.HeaderIntegrityVerifierStub{},
	}
}
