package factory

import (
	"github.com/ElrondNetwork/elrond-go/core/check"
	"github.com/ElrondNetwork/elrond-go/data/typeConverters"
	"github.com/ElrondNetwork/elrond-go/hashing"
	"github.com/ElrondNetwork/elrond-go/marshal"
	"github.com/ElrondNetwork/elrond-go/process"
	"github.com/ElrondNetwork/elrond-go/process/block/interceptedBlocks"
	"github.com/ElrondNetwork/elrond-go/sharding"
)

type interceptedMetaHeaderDataFactory struct {
	marshalizer       marshal.Marshalizer
	hasher            hashing.Hasher
	shardCoordinator  sharding.Coordinator
	headerSigVerifier process.InterceptedHeaderSigVerifier
	chainID           []byte
	validityAttester  process.ValidityAttester
	epochStartTrigger process.EpochStartTriggerHandler
	nonceConverter    typeConverters.Uint64ByteSliceConverter
}

// NewInterceptedMetaHeaderDataFactory creates an instance of interceptedMetaHeaderDataFactory
func NewInterceptedMetaHeaderDataFactory(argument *ArgInterceptedDataFactory) (*interceptedMetaHeaderDataFactory, error) {
	if argument == nil {
		return nil, process.ErrNilArgumentStruct
	}
	if check.IfNil(argument.ProtoMarshalizer) {
		return nil, process.ErrNilMarshalizer
	}
	if check.IfNil(argument.TxSignMarshalizer) {
		return nil, process.ErrNilMarshalizer
	}
	if check.IfNil(argument.Hasher) {
		return nil, process.ErrNilHasher
	}
	if check.IfNil(argument.ShardCoordinator) {
		return nil, process.ErrNilShardCoordinator
	}
	if check.IfNil(argument.HeaderSigVerifier) {
		return nil, process.ErrNilHeaderSigVerifier
	}
	if check.IfNil(argument.EpochStartTrigger) {
		return nil, process.ErrNilEpochStartTrigger
	}
	if len(argument.ChainID) == 0 {
		return nil, process.ErrInvalidChainID
	}
	if check.IfNil(argument.ValidityAttester) {
		return nil, process.ErrNilValidityAttester
	}
	if check.IfNil(argument.NonceConverter) {
		return nil, process.ErrNilUint64Converter
	}

	return &interceptedMetaHeaderDataFactory{
		marshalizer:       argument.ProtoMarshalizer,
		hasher:            argument.Hasher,
		shardCoordinator:  argument.ShardCoordinator,
		headerSigVerifier: argument.HeaderSigVerifier,
		chainID:           argument.ChainID,
		validityAttester:  argument.ValidityAttester,
		epochStartTrigger: argument.EpochStartTrigger,
		nonceConverter:    argument.NonceConverter,
	}, nil
}

// Create creates instances of InterceptedData by unmarshalling provided buffer
func (imhdf *interceptedMetaHeaderDataFactory) Create(buff []byte) (process.InterceptedData, error) {
	arg := &interceptedBlocks.ArgInterceptedBlockHeader{
		HdrBuff:           buff,
		Marshalizer:       imhdf.marshalizer,
		Hasher:            imhdf.hasher,
		ShardCoordinator:  imhdf.shardCoordinator,
		HeaderSigVerifier: imhdf.headerSigVerifier,
		ChainID:           imhdf.chainID,
		ValidityAttester:  imhdf.validityAttester,
		EpochStartTrigger: imhdf.epochStartTrigger,
		NonceConverter:    imhdf.nonceConverter,
	}

	return interceptedBlocks.NewInterceptedMetaHeader(arg)
}

// IsInterfaceNil returns true if there is no value under the interface
func (imhdf *interceptedMetaHeaderDataFactory) IsInterfaceNil() bool {
	return imhdf == nil
}