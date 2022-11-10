//go:generate protoc -I=. -I=$GOPATH/src -I=$GOPATH/src/github.com/ElrondNetwork/protobuf/protobuf  --gogoslick_out=. userAccountData.proto
package state

import (
	"bytes"
	"math/big"

	"github.com/ElrondNetwork/elrond-go-core/core/check"
	"github.com/ElrondNetwork/elrond-go-core/hashing"
	"github.com/ElrondNetwork/elrond-go-core/marshal"
	"github.com/ElrondNetwork/elrond-go/common"
)

var _ UserAccountHandler = (*userAccount)(nil)

// Account is the struct used in serialization/deserialization
type userAccount struct {
	*baseAccount
	UserAccountData
}

var zero = big.NewInt(0)

// NewEmptyUserAccount creates a new empty instance of userAccount
func NewEmptyUserAccount() *userAccount {
	return &userAccount{
		baseAccount: &baseAccount{},
		UserAccountData: UserAccountData{
			DeveloperReward: big.NewInt(0),
			Balance:         big.NewInt(0),
		},
	}
}

// ArgsAccountCreation holds the arguments needed to create a new instance of userAccount
type ArgsAccountCreation struct {
	Hasher              hashing.Hasher
	Marshaller          marshal.Marshalizer
	EnableEpochsHandler common.EnableEpochsHandler
}

// NewUserAccount creates a new instance of userAccount
func NewUserAccount(
	address []byte,
	args ArgsAccountCreation,
) (*userAccount, error) {
	if len(address) == 0 {
		return nil, ErrNilAddress
	}
	if check.IfNil(args.Marshaller) {
		return nil, ErrNilMarshalizer
	}
	if check.IfNil(args.Hasher) {
		return nil, ErrNilHasher
	}
	if check.IfNil(args.EnableEpochsHandler) {
		return nil, ErrNilEnableEpochsHandler
	}

	tdt, err := NewTrackableDataTrie(address, nil, args.Hasher, args.Marshaller)
	if err != nil {
		return nil, err
	}

	return &userAccount{
		baseAccount: &baseAccount{
			address:         address,
			dataTrieTracker: tdt,
		},
		UserAccountData: UserAccountData{
			DeveloperReward: big.NewInt(0),
			Balance:         big.NewInt(0),
			Address:         address,
		},
	}, nil
}

// SetUserName sets the users name
func (a *userAccount) SetUserName(userName []byte) {
	a.UserName = make([]byte, 0, len(userName))
	a.UserName = append(a.UserName, userName...)
}

// AddToBalance adds new value to balance
func (a *userAccount) AddToBalance(value *big.Int) error {
	newBalance := big.NewInt(0).Add(a.Balance, value)
	if newBalance.Cmp(zero) < 0 {
		return ErrInsufficientFunds
	}

	a.Balance = newBalance
	return nil
}

// SubFromBalance subtracts new value from balance
func (a *userAccount) SubFromBalance(value *big.Int) error {
	newBalance := big.NewInt(0).Sub(a.Balance, value)
	if newBalance.Cmp(zero) < 0 {
		return ErrInsufficientFunds
	}

	a.Balance = newBalance
	return nil
}

// GetBalance returns the actual balance from the account
func (a *userAccount) GetBalance() *big.Int {
	return big.NewInt(0).Set(a.Balance)
}

// ClaimDeveloperRewards returns the accumulated developer rewards and sets it to 0 in the account
func (a *userAccount) ClaimDeveloperRewards(sndAddress []byte) (*big.Int, error) {
	if !bytes.Equal(sndAddress, a.OwnerAddress) {
		return nil, ErrOperationNotPermitted
	}

	oldValue := big.NewInt(0).Set(a.DeveloperReward)
	a.DeveloperReward = big.NewInt(0)

	return oldValue, nil
}

// AddToDeveloperReward adds new value to developer reward
func (a *userAccount) AddToDeveloperReward(value *big.Int) {
	a.DeveloperReward = big.NewInt(0).Add(a.DeveloperReward, value)
}

// GetDeveloperReward returns the actual developer reward from the account
func (a *userAccount) GetDeveloperReward() *big.Int {
	return big.NewInt(0).Set(a.DeveloperReward)
}

// ChangeOwnerAddress changes the owner account if operation is permitted
func (a *userAccount) ChangeOwnerAddress(sndAddress []byte, newAddress []byte) error {
	if !bytes.Equal(sndAddress, a.OwnerAddress) {
		return ErrOperationNotPermitted
	}
	if len(newAddress) != len(a.address) {
		return ErrInvalidAddressLength
	}

	a.OwnerAddress = newAddress

	return nil
}

// SetOwnerAddress sets the owner address of an account,
func (a *userAccount) SetOwnerAddress(address []byte) {
	a.OwnerAddress = address
}

// IncreaseNonce adds the given value to the current nonce
func (a *userAccount) IncreaseNonce(value uint64) {
	a.Nonce = a.Nonce + value
}

// SetCodeHash sets the code hash associated with the account
func (a *userAccount) SetCodeHash(codeHash []byte) {
	a.CodeHash = codeHash
}

// SetRootHash sets the root hash associated with the account
func (a *userAccount) SetRootHash(roothash []byte) {
	a.RootHash = roothash
}

// SetCodeMetadata sets the code metadata
func (a *userAccount) SetCodeMetadata(codeMetadata []byte) {
	a.CodeMetadata = codeMetadata
}

// IsInterfaceNil returns true if there is no value under the interface
func (a *userAccount) IsInterfaceNil() bool {
	return a == nil
}
