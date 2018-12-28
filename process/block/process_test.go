package block_test

import (
	"math/big"
	"testing"

	"github.com/ElrondNetwork/elrond-go-sandbox/data/block"
	"github.com/ElrondNetwork/elrond-go-sandbox/data/blockchain"
	"github.com/ElrondNetwork/elrond-go-sandbox/data/shardedData"
	"github.com/ElrondNetwork/elrond-go-sandbox/data/transaction"
	"github.com/ElrondNetwork/elrond-go-sandbox/process"
	blproc "github.com/ElrondNetwork/elrond-go-sandbox/process/block"
	"github.com/ElrondNetwork/elrond-go-sandbox/process/mock"
	"github.com/ElrondNetwork/elrond-go-sandbox/storage"
	"github.com/stretchr/testify/assert"
)

var testCacherConfig = storage.CacheConfig{
	Size: 1000,
	Type: storage.LRUCache,
}

func blockchainConfig() *blockchain.Config {
	cacher := storage.CacheConfig{Type: storage.LRUCache, Size: 100}
	bloom := storage.BloomConfig{Size: 2048, HashFunc: []storage.HasherType{storage.Keccak, storage.Blake2b, storage.Fnv}}
	persisterTxBlockBodyStorage := storage.DBConfig{Type: storage.LvlDB, FilePath: "TxBlockBodyStorage"}
	persisterStateBlockBodyStorage := storage.DBConfig{Type: storage.LvlDB, FilePath: "StateBlockBodyStorage"}
	persisterPeerBlockBodyStorage := storage.DBConfig{Type: storage.LvlDB, FilePath: "PeerBlockBodyStorage"}
	persisterBlockHeaderStorage := storage.DBConfig{Type: storage.LvlDB, FilePath: "BlockHeaderStorage"}
	persisterTxStorage := storage.DBConfig{Type: storage.LvlDB, FilePath: "TxStorage"}
	return &blockchain.Config{
		TxBlockBodyStorage:    storage.UnitConfig{CacheConf: cacher, DBConf: persisterTxBlockBodyStorage, BloomConf: bloom},
		StateBlockBodyStorage: storage.UnitConfig{CacheConf: cacher, DBConf: persisterStateBlockBodyStorage, BloomConf: bloom},
		PeerBlockBodyStorage:  storage.UnitConfig{CacheConf: cacher, DBConf: persisterPeerBlockBodyStorage, BloomConf: bloom},
		BlockHeaderStorage:    storage.UnitConfig{CacheConf: cacher, DBConf: persisterBlockHeaderStorage, BloomConf: bloom},
		TxStorage:             storage.UnitConfig{CacheConf: cacher, DBConf: persisterTxStorage, BloomConf: bloom},
		TxPoolStorage:         cacher,
		TxBadBlockBodyCache:   cacher,
	}
}

func createBlockChainFromConfig(blConfig *blockchain.Config) (*blockchain.BlockChain, error) {
	var headerUnit, peerBlockUnit, stateBlockUnit, txBlockUnit, txUnit *storage.Unit

	txBadBlockCache, err := storage.NewCache(
		blConfig.TxBadBlockBodyCache.Type,
		blConfig.TxBadBlockBodyCache.Size)

	if err == nil {
		txUnit, err = storage.NewStorageUnitFromConf(
			blConfig.TxStorage.CacheConf,
			blConfig.TxStorage.DBConf,
			blConfig.TxStorage.BloomConf)
	}

	if err == nil {
		txBlockUnit, err = storage.NewStorageUnitFromConf(
			blConfig.TxBlockBodyStorage.CacheConf,
			blConfig.TxBlockBodyStorage.DBConf,
			blConfig.TxBlockBodyStorage.BloomConf)
	}

	if err == nil {
		stateBlockUnit, err = storage.NewStorageUnitFromConf(
			blConfig.StateBlockBodyStorage.CacheConf,
			blConfig.StateBlockBodyStorage.DBConf,
			blConfig.StateBlockBodyStorage.BloomConf)
	}

	if err == nil {
		peerBlockUnit, err = storage.NewStorageUnitFromConf(
			blConfig.PeerBlockBodyStorage.CacheConf,
			blConfig.PeerBlockBodyStorage.DBConf,
			blConfig.PeerBlockBodyStorage.BloomConf)
	}

	if err == nil {
		headerUnit, err = storage.NewStorageUnitFromConf(
			blConfig.BlockHeaderStorage.CacheConf,
			blConfig.BlockHeaderStorage.DBConf,
			blConfig.BlockHeaderStorage.BloomConf)
	}

	if err == nil {
		blockChain, err := blockchain.NewBlockChain(
			txBadBlockCache,
			txUnit,
			txBlockUnit,
			stateBlockUnit,
			peerBlockUnit,
			headerUnit)

		return blockChain, err
	}

	// cleanup
	if err != nil {
		if headerUnit != nil {
			_ = headerUnit.DestroyUnit()
		}
		if peerBlockUnit != nil {
			_ = peerBlockUnit.DestroyUnit()
		}
		if stateBlockUnit != nil {
			_ = stateBlockUnit.DestroyUnit()
		}
		if txBlockUnit != nil {
			_ = txBlockUnit.DestroyUnit()
		}
		if txUnit != nil {
			_ = txUnit.DestroyUnit()
		}
	}
	return nil, err
}

func TestNewBlockProcessor(t *testing.T) {
	tp, err := shardedData.NewShardedData(testCacherConfig)
	assert.Nil(t, err)

	be := blproc.NewBlockProcessor(
		tp,
		&mock.HasherMock{},
		&mock.MarshalizerMock{},
		&mock.TxProcessorMock{},
		nil,
		mock.NewOneShardCoordinatorMock())

	assert.NotNil(t, be)
}

func TestBlockProc_GetTransactionFromPool(t *testing.T) {
	tp, err := shardedData.NewShardedData(testCacherConfig)
	assert.Nil(t, err)

	be := blproc.NewBlockProcessor(
		tp, &mock.HasherMock{},
		&mock.MarshalizerMock{},
		&mock.TxProcessorMock{},
		nil,
		mock.NewOneShardCoordinatorMock())

	txHash := []byte("tx1_hash")

	tx := be.GetTransactionFromPool(1, txHash)
	assert.Nil(t, tp.ShardDataStore(1))
	assert.Nil(t, tx)

	tp.NewShardStore(1)

	tx = be.GetTransactionFromPool(1, txHash)
	assert.NotNil(t, tp.ShardDataStore(1))
	assert.Nil(t, tx)

	testedNonce := uint64(1)
	tp.AddData(txHash, &transaction.Transaction{Nonce: testedNonce}, 1)

	tx = be.GetTransactionFromPool(1, txHash)
	assert.NotNil(t, tp.ShardDataStore(1))
	assert.NotNil(t, tx)
	assert.Equal(t, testedNonce, tx.Nonce)
}

func TestBlockProc_RequestTransactionFromNetwork(t *testing.T) {
	tp, err := shardedData.NewShardedData(testCacherConfig)
	assert.Nil(t, err)

	be := blproc.NewBlockProcessor(
		tp, &mock.HasherMock{},
		&mock.MarshalizerMock{},
		&mock.TxProcessorMock{},
		nil,
		mock.NewOneShardCoordinatorMock())

	shardId := uint32(1)
	txHash1 := []byte("tx1_hash1")

	blk := block.TxBlockBody{}
	mBlocks := make([]block.MiniBlock, 0)
	txHashes := make([][]byte, 0)
	txHashes = append(txHashes, txHash1)
	mBlk := block.MiniBlock{ShardID: shardId, TxHashes: txHashes}
	mBlocks = append(mBlocks, mBlk)
	blk.MiniBlocks = mBlocks
	tx1 := &transaction.Transaction{Nonce: 7}
	tp.AddData(txHash1, tx1, 1)

	be.RequestTransactionFromNetwork(&blk)
	be.WaitForTxHashes()
	tx, _ := tp.ShardStore(shardId).DataStore.Get(txHash1)

	assert.Equal(t, tx1, tx)
}

func TestBlockProcessor_ProcessBlockWithNilTxBlockBodyShouldErr(t *testing.T) {
	tp, err := shardedData.NewShardedData(testCacherConfig)
	assert.Nil(t, err)
	tpm := mock.TxProcessorMock{}
	// set accounts dirty
	JournalLen := func() int { return 3 }

	blkc, _ := createBlockChainFromConfig(blockchainConfig())

	hdr := block.Header{
		Nonce:         0,
		BlockBodyHash: []byte("bodyHash"),
		PubKeysBitmap: []byte("0100101"),
		PrevHash:      []byte(""),
		Signature:     []byte("signature"),
		Commitment:    []byte("commitment"),
	}

	// cleanup after tests
	defer func() {
		_ = blkc.Destroy()
	}()

	be := blproc.NewBlockProcessor(
		tp, &mock.HasherMock{},
		&mock.MarshalizerMock{},
		&tpm,
		&mock.AccountsStub{JournalLenCalled: JournalLen},
		mock.NewOneShardCoordinatorMock(),
	)

	// should return err
	err = be.ProcessBlock(blkc, &hdr, nil)
	assert.NotNil(t, err)
	assert.Equal(t, process.ErrNilTxBlockBody, err)
}

func TestBlockProc_ProcessBlockWithDirtyAccountShouldErr(t *testing.T) {
	tp, err := shardedData.NewShardedData(testCacherConfig)
	assert.Nil(t, err)
	tpm := mock.TxProcessorMock{}
	// set accounts dirty
	JournalLen := func() int { return 3 }

	blkc, _ := createBlockChainFromConfig(blockchainConfig())

	hdr := block.Header{
		Nonce:         0,
		BlockBodyHash: []byte("bodyHash"),
		PubKeysBitmap: []byte("0100101"),
		PrevHash:      []byte(""),
		Signature:     []byte("signature"),
		Commitment:    []byte("commitment"),
	}

	miniblocks := make([]block.MiniBlock, 0)

	txBody := block.TxBlockBody{
		StateBlockBody: block.StateBlockBody{
			RootHash: []byte("root hash"),
			ShardID:  0,
		},
		MiniBlocks: miniblocks,
	}

	// cleanup after tests
	defer func() {
		_ = blkc.Destroy()
	}()

	be := blproc.NewBlockProcessor(
		tp, &mock.HasherMock{},
		&mock.MarshalizerMock{},
		&tpm,
		&mock.AccountsStub{JournalLenCalled: JournalLen},
		mock.NewOneShardCoordinatorMock(),
	)

	// should return err
	err = be.ProcessBlock(blkc, &hdr, &txBody)
	assert.NotNil(t, err)
	assert.Equal(t, err, process.ErrAccountStateDirty)
}

func TestBlockProcessor_ProcessBlockWithInvalidTransactionShouldErr(t *testing.T) {
	tp, err := shardedData.NewShardedData(testCacherConfig)
	assert.Nil(t, err)
	txHash := []byte("tx_hash1")
	tp.AddData(txHash, &transaction.Transaction{Nonce: 1}, 0)

	// invalid transaction
	txProcess := func(transaction *transaction.Transaction, round int32) error {
		return process.ErrHigherNonceInTransaction
	}

	tpm := mock.TxProcessorMock{ProcessTransactionCalled: txProcess}
	blkc, _ := createBlockChainFromConfig(blockchainConfig())
	hdr := block.Header{
		Nonce:         0,
		PrevHash:      []byte(""),
		Signature:     []byte("signature"),
		PubKeysBitmap: []byte("00110"),
		BlockBodyHash: []byte("bodyHash"),
		ShardId:       0,
		Commitment:    []byte("commitment"),
	}
	miniblocks := make([]block.MiniBlock, 0)

	txHashes := make([][]byte, 0)
	txHashes = append(txHashes, txHash)

	miniblock := block.MiniBlock{
		ShardID:  0,
		TxHashes: txHashes,
	}
	miniblocks = append(miniblocks, miniblock)

	txBody := block.TxBlockBody{
		StateBlockBody: block.StateBlockBody{
			RootHash: []byte("root hash"),
			ShardID:  0,
		},
		MiniBlocks: miniblocks,
	}

	// cleanup after tests
	defer func() {
		_ = blkc.Destroy()
	}()

	// set accounts dirty
	journalLen := func() int { return 0 }
	// set revertToSnapshot
	revertToSnapshot := func(snapshot int) error { return nil }

	be := blproc.NewBlockProcessor(
		tp, &mock.HasherMock{},
		&mock.MarshalizerMock{},
		&tpm,
		&mock.AccountsStub{
			JournalLenCalled:       journalLen,
			RevertToSnapshotCalled: revertToSnapshot,
		},
		mock.NewOneShardCoordinatorMock(),
	)

	// should return err
	err = be.ProcessBlock(blkc, &hdr, &txBody)
	assert.Equal(t, process.ErrHigherNonceInTransaction, err)
}

func TestBlockProc_CreateTxBlockBodyWithDirtyAccStateShouldErr(t *testing.T) {
	tp, err := shardedData.NewShardedData(testCacherConfig)
	assert.Nil(t, err)
	tpm := mock.TxProcessorMock{}
	JournalLen := func() int { return 3 }

	be := blproc.NewBlockProcessor(
		tp, &mock.HasherMock{},
		&mock.MarshalizerMock{},
		&tpm,
		&mock.AccountsStub{JournalLenCalled: JournalLen},
		mock.NewOneShardCoordinatorMock(),
	)

	bl, err := be.CreateTxBlockBody(0, 100, 0, func() bool { return true })

	// nil block
	assert.Nil(t, bl)
	// error
	assert.Equal(t, process.ErrAccountStateDirty, err)
}

func TestBlockProcessor_CreateTxBlockBodyWithNoTimeShouldEmptyBlock(t *testing.T) {
	tp, err := shardedData.NewShardedData(testCacherConfig)
	assert.Nil(t, err)
	tpm := mock.TxProcessorMock{}
	journalLen := func() int { return 0 }
	rootHashfunc := func() []byte { return []byte("roothash") }

	be := blproc.NewBlockProcessor(
		tp, &mock.HasherMock{},
		&mock.MarshalizerMock{},
		&tpm,
		&mock.AccountsStub{
			JournalLenCalled: journalLen,
			RootHashCalled:   rootHashfunc,
		},
		mock.NewOneShardCoordinatorMock(),
	)

	haveTime := func() bool {
		return false
	}

	bl, err := be.CreateTxBlockBody(0, 100, 0, haveTime)

	// no error
	assert.Nil(t, err)
	// no miniblocks
	assert.Equal(t, len(bl.MiniBlocks), 0)
}

func TestBlockProcessor_CreateTxBlockBodyOK(t *testing.T) {
	tp, err := shardedData.NewShardedData(testCacherConfig)
	assert.Nil(t, err)
	//process transaction. return nil for no error
	procTx := func(transaction *transaction.Transaction, round int32) error {
		return nil
	}

	tpm := mock.TxProcessorMock{
		ProcessTransactionCalled: procTx,
	}

	journalLen := func() int { return 0 }
	rootHashfunc := func() []byte { return []byte("roothash") }

	haveTime := func() bool {
		return true
	}

	be := blproc.NewBlockProcessor(
		tp, &mock.HasherMock{},
		&mock.MarshalizerMock{},
		&tpm,
		&mock.AccountsStub{
			JournalLenCalled: journalLen,
			RootHashCalled:   rootHashfunc,
		},
		mock.NewOneShardCoordinatorMock(),
	)

	blk, err := be.CreateTxBlockBody(0, 100, 0, haveTime)

	assert.NotNil(t, blk)
	assert.Nil(t, err)
}

func TestBlockProcessor_CreateGenesisBlockBodyWithNilTxProcessorShouldPanic(t *testing.T) {
	tp, err := shardedData.NewShardedData(testCacherConfig)
	assert.Nil(t, err)

	be := blproc.NewBlockProcessor(
		tp, nil,
		nil,
		nil,
		nil,
		mock.NewOneShardCoordinatorMock(),
	)

	createGenesis := func() {
		be.CreateGenesisBlockBody(nil, 0)
	}

	assert.Panics(t, createGenesis)
}

func TestBlockProcessor_CreateGenesisBlockBodyWithFailSetBalanceShouldPanic(t *testing.T) {
	tp, err := shardedData.NewShardedData(testCacherConfig)
	assert.Nil(t, err)

	txProcess := func(transaction *transaction.Transaction, round int32) error {
		return nil
	}

	setBalances := func(accBalance map[string]big.Int) (rootHash []byte, err error) {
		return nil, process.ErrAccountStateDirty
	}

	txProc := mock.TxProcessorMock{
		ProcessTransactionCalled: txProcess,
		SetBalancesToTrieCalled:  setBalances,
	}

	be := blproc.NewBlockProcessor(
		tp, nil,
		nil,
		&txProc,
		nil,
		mock.NewOneShardCoordinatorMock(),
	)

	createGenesis := func() {
		be.CreateGenesisBlockBody(nil, 0)
	}

	assert.Panics(t, createGenesis)
}

func TestBlockProcessor_CreateGenesisBlockBodyOK(t *testing.T) {
	tp, err := shardedData.NewShardedData(testCacherConfig)
	assert.Nil(t, err)

	txProcess := func(transaction *transaction.Transaction, round int32) error {
		return nil
	}

	setBalances := func(accBalance map[string]big.Int) (rootHash []byte, err error) {
		return []byte("stateRootHash"), nil
	}

	txProc := mock.TxProcessorMock{
		ProcessTransactionCalled: txProcess,
		SetBalancesToTrieCalled:  setBalances,
	}

	be := blproc.NewBlockProcessor(
		tp, nil,
		nil,
		&txProc,
		nil,
		mock.NewOneShardCoordinatorMock(),
	)

	stBlock := be.CreateGenesisBlockBody(nil, 0)
	assert.NotNil(t, stBlock)
	assert.Equal(t, stBlock.RootHash, []byte("stateRootHash"))
}

func TestBlockProcessor_RemoveBlockTxsFromPoolNilBlockShouldErr(t *testing.T) {
	tp, err := shardedData.NewShardedData(testCacherConfig)
	assert.Nil(t, err)

	be := blproc.NewBlockProcessor(
		tp, nil,
		nil,
		nil,
		nil,
		mock.NewOneShardCoordinatorMock(),
	)

	err = be.RemoveBlockTxsFromPool(nil)

	assert.NotNil(t, err)
	assert.Equal(t, err, process.ErrNilTxBlockBody)
}

func TestBlockProcessor_RemoveBlockTxsFromPoolOK(t *testing.T) {
	tp, err := shardedData.NewShardedData(testCacherConfig)
	assert.Nil(t, err)

	be := blproc.NewBlockProcessor(
		tp, nil,
		nil,
		nil,
		nil,
		mock.NewOneShardCoordinatorMock(),
	)
	miniblocks := make([]block.MiniBlock, 0)

	txHash := []byte("txHash")
	txHashes := make([][]byte, 0)
	txHashes = append(txHashes, txHash)

	miniblock := block.MiniBlock{
		ShardID:  0,
		TxHashes: txHashes,
	}

	miniblocks = append(miniblocks, miniblock)

	txBody := block.TxBlockBody{
		StateBlockBody: block.StateBlockBody{},
		MiniBlocks:     miniblocks,
	}

	err = be.RemoveBlockTxsFromPool(&txBody)

	assert.Nil(t, err)
}