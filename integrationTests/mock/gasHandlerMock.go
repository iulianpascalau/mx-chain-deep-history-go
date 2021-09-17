package mock

import (
	"github.com/ElrondNetwork/elrond-go-core/data"
	"github.com/ElrondNetwork/elrond-go-core/data/block"
)

// GasHandlerMock -
type GasHandlerMock struct {
	InitCalled                           func()
	SetGasConsumedCalled                 func(gasConsumed uint64, hash []byte)
	SetGasRefundedCalled                 func(gasRefunded uint64, hash []byte)
	GasConsumedCalled                    func(hash []byte) uint64
	GasRefundedCalled                    func(hash []byte) uint64
	TotalGasConsumedCalled               func() uint64
	TotalGasRefundedCalled               func() uint64
	RemoveGasConsumedCalled              func(hashes [][]byte)
	RemoveGasRefundedCalled              func(hashes [][]byte)
	ComputeGasConsumedByMiniBlockCalled  func(miniBlock *block.MiniBlock, mapHashTx map[string]data.TransactionHandler) (uint64, uint64, error)
	ComputeGasConsumedByTxCalled         func(txSenderShardId uint32, txReceiverSharedId uint32, txHandler data.TransactionHandler) (uint64, uint64, error)
	SetTotalGasConsumedInSelfShardCalled func(gasConsumed uint64)
	GetTotalGasConsumedInSelfShardCalled func() uint64
}

// Init -
func (ghm *GasHandlerMock) Init() {
	ghm.InitCalled()
}

// SetGasConsumed -
func (ghm *GasHandlerMock) SetGasConsumed(gasConsumed uint64, hash []byte) {
	ghm.SetGasConsumedCalled(gasConsumed, hash)
}

// SetGasRefunded -
func (ghm *GasHandlerMock) SetGasRefunded(gasRefunded uint64, hash []byte) {
	ghm.SetGasRefundedCalled(gasRefunded, hash)
}

// GasConsumed -
func (ghm *GasHandlerMock) GasConsumed(hash []byte) uint64 {
	return ghm.GasConsumedCalled(hash)
}

// GasRefunded -
func (ghm *GasHandlerMock) GasRefunded(hash []byte) uint64 {
	return ghm.GasRefundedCalled(hash)
}

// TotalGasConsumed -
func (ghm *GasHandlerMock) TotalGasConsumed() uint64 {
	return ghm.TotalGasConsumedCalled()
}

// TotalGasRefunded -
func (ghm *GasHandlerMock) TotalGasRefunded() uint64 {
	return ghm.TotalGasRefundedCalled()
}

// RemoveGasConsumed -
func (ghm *GasHandlerMock) RemoveGasConsumed(hashes [][]byte) {
	ghm.RemoveGasConsumedCalled(hashes)
}

// RemoveGasRefunded -
func (ghm *GasHandlerMock) RemoveGasRefunded(hashes [][]byte) {
	ghm.RemoveGasRefundedCalled(hashes)
}

// ComputeGasConsumedByMiniBlock -
func (ghm *GasHandlerMock) ComputeGasConsumedByMiniBlock(miniBlock *block.MiniBlock, mapHashTx map[string]data.TransactionHandler) (uint64, uint64, error) {
	return ghm.ComputeGasConsumedByMiniBlockCalled(miniBlock, mapHashTx)
}

// ComputeGasConsumedByTx -
func (ghm *GasHandlerMock) ComputeGasConsumedByTx(txSenderShardId uint32, txReceiverShardId uint32, txHandler data.TransactionHandler) (uint64, uint64, error) {
	return ghm.ComputeGasConsumedByTxCalled(txSenderShardId, txReceiverShardId, txHandler)
}

// SetTotalGasConsumedInSelfShard -
func (ghm *GasHandlerMock) SetTotalGasConsumedInSelfShard(gasConsumed uint64) {
	if ghm.SetTotalGasConsumedInSelfShardCalled != nil {
		ghm.SetTotalGasConsumedInSelfShardCalled(gasConsumed)
	}
}

// GetTotalGasConsumedInSelfShard -
func (ghm *GasHandlerMock) GetTotalGasConsumedInSelfShard() uint64 {
	if ghm.GetTotalGasConsumedInSelfShardCalled != nil {
		return ghm.GetTotalGasConsumedInSelfShardCalled()
	}

	return 0
}

// IsInterfaceNil -
func (ghm *GasHandlerMock) IsInterfaceNil() bool {
	return ghm == nil
}
