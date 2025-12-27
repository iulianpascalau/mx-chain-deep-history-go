package network

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/multiversx/mx-chain-communication-go/p2p"
	"github.com/multiversx/mx-chain-communication-go/p2p/libp2p/crypto"
	"github.com/multiversx/mx-chain-core-go/core"
	"github.com/multiversx/mx-chain-core-go/core/check"
	"github.com/multiversx/mx-chain-core-go/hashing/blake2b"
)

const virtualAddressTemplate = "/virtual/p2p/%s"

var (
	p2pInstanceCreator, _  = crypto.NewIdentityGenerator(log)
	hasher                 = blake2b.NewBlake2b()
	errTopicAlreadyCreated = errors.New("topic already created")
	errNilMessageProcessor = errors.New("nil message processor")
	errTopicNotCreated     = errors.New("topic not created")
	errTopicHasProcessor   = errors.New("there is already a message processor for provided topic and identifier")
	errInvalidSignature    = errors.New("invalid signature")
	errMessengerIsClosed   = errors.New("messenger is closed")
)

type disabledMessenger struct {
	mutIsClosed  sync.RWMutex
	isClosed     bool
	mutOperation sync.RWMutex
	topics       map[string]map[string]p2p.MessageProcessor
	pid          core.PeerID
}

// NewDisabledMessenger creates a new disabled network messenger
func NewDisabledMessenger() (*disabledMessenger, error) {
	_, pid, err := p2pInstanceCreator.CreateRandomP2PIdentity()
	if err != nil {
		return nil, err
	}

	messenger := &disabledMessenger{
		topics: make(map[string]map[string]p2p.MessageProcessor),
		pid:    pid,
	}

	log.Debug("created syncedMessenger", "pid", pid.Pretty())

	return messenger, nil
}

// HasCompatibleProtocolID returns true
func (messenger *disabledMessenger) HasCompatibleProtocolID(_ string) bool {
	return true
}

// ProcessReceivedMessage does nothing and returns nil
func (messenger *disabledMessenger) ProcessReceivedMessage(_ p2p.MessageP2P, _ core.PeerID, _ p2p.MessageHandler) ([]byte, error) {
	return nil, nil
}

// CreateTopic will create a topic for receiving data
func (messenger *disabledMessenger) CreateTopic(name string, _ bool) error {
	if messenger.closed() {
		return errMessengerIsClosed
	}

	messenger.mutOperation.Lock()
	defer messenger.mutOperation.Unlock()

	_, found := messenger.topics[name]
	if found {
		return fmt.Errorf("programming error in syncedMessenger.CreateTopic, %w for topic %s", errTopicAlreadyCreated, name)
	}

	messenger.topics[name] = make(map[string]p2p.MessageProcessor)

	return nil
}

// HasTopic returns true if the topic was registered
func (messenger *disabledMessenger) HasTopic(name string) bool {
	messenger.mutOperation.RLock()
	defer messenger.mutOperation.RUnlock()

	_, found := messenger.topics[name]

	return found
}

// RegisterMessageProcessor will try to register a message processor on the provided topic & identifier
func (messenger *disabledMessenger) RegisterMessageProcessor(topic string, identifier string, handler p2p.MessageProcessor) error {
	if messenger.closed() {
		return errMessengerIsClosed
	}
	if check.IfNil(handler) {
		return fmt.Errorf("programming error in syncedMessenger.RegisterMessageProcessor, "+
			"%w for topic %s and identifier %s", errNilMessageProcessor, topic, identifier)
	}

	messenger.mutOperation.Lock()
	defer messenger.mutOperation.Unlock()

	handlers, found := messenger.topics[topic]
	if !found {
		handlers = make(map[string]p2p.MessageProcessor)
		messenger.topics[topic] = handlers
	}

	_, found = handlers[identifier]
	if found {
		return fmt.Errorf("programming error in syncedMessenger.RegisterMessageProcessor, %w, topic %s, identifier %s",
			errTopicHasProcessor, topic, identifier)
	}

	handlers[identifier] = handler

	return nil
}

// UnregisterAllMessageProcessors will unregister all message processors
func (messenger *disabledMessenger) UnregisterAllMessageProcessors() error {
	messenger.mutOperation.Lock()
	defer messenger.mutOperation.Unlock()

	for topic := range messenger.topics {
		messenger.topics[topic] = make(map[string]p2p.MessageProcessor)
	}

	return nil
}

// UnregisterMessageProcessor will unregister the message processor for the provided topic and identifier
func (messenger *disabledMessenger) UnregisterMessageProcessor(topic string, identifier string) error {
	messenger.mutOperation.Lock()
	defer messenger.mutOperation.Unlock()

	handlers, found := messenger.topics[topic]
	if !found {
		return fmt.Errorf("programming error in syncedMessenger.UnregisterMessageProcessor, %w for topic %s",
			errTopicNotCreated, topic)
	}

	delete(handlers, identifier)

	return nil
}

// Broadcast will broadcast the provided buffer on the topic in a synchronous manner
func (messenger *disabledMessenger) Broadcast(_ string, _ []byte) {}

// BroadcastOnChannel calls the Broadcast method
func (messenger *disabledMessenger) BroadcastOnChannel(_ string, topic string, buff []byte) {
	messenger.Broadcast(topic, buff)
}

// BroadcastUsingPrivateKey calls the Broadcast method
func (messenger *disabledMessenger) BroadcastUsingPrivateKey(topic string, buff []byte, _ core.PeerID, _ []byte) {
	messenger.Broadcast(topic, buff)
}

// BroadcastOnChannelUsingPrivateKey calls the Broadcast method
func (messenger *disabledMessenger) BroadcastOnChannelUsingPrivateKey(_ string, topic string, buff []byte, _ core.PeerID, _ []byte) {
	messenger.Broadcast(topic, buff)
}

// SendToConnectedPeer will send the message to the peer
func (messenger *disabledMessenger) SendToConnectedPeer(topic string, buff []byte, peerID core.PeerID) error {
	if messenger.closed() {
		return errMessengerIsClosed
	}

	if !messenger.HasTopic(topic) {
		return nil
	}

	log.Trace("syncedMessenger.SendToConnectedPeer",
		"from", messenger.pid.Pretty(),
		"to", peerID.Pretty(),
		"data", buff)

	return nil
}

// UnJoinAllTopics will unjoin all topics
func (messenger *disabledMessenger) UnJoinAllTopics() error {
	messenger.mutOperation.Lock()
	defer messenger.mutOperation.Unlock()

	messenger.topics = make(map[string]map[string]p2p.MessageProcessor)

	return nil
}

// Bootstrap does nothing and returns nil
func (messenger *disabledMessenger) Bootstrap() error {
	return nil
}

// Peers returns the network's peer ID
func (messenger *disabledMessenger) Peers() []core.PeerID {
	return make([]core.PeerID, 0)
}

// Addresses returns the addresses this messenger was bound to. It returns a virtual address
func (messenger *disabledMessenger) Addresses() []string {
	return []string{fmt.Sprintf(virtualAddressTemplate, messenger.pid.Pretty())}
}

// ConnectToPeer does nothing and returns nil
func (messenger *disabledMessenger) ConnectToPeer(_ string) error {
	return nil
}

// IsConnected returns true if the peer ID is found on the network
func (messenger *disabledMessenger) IsConnected(_ core.PeerID) bool {
	return false
}

// ConnectedPeers returns the same list as the function Peers
func (messenger *disabledMessenger) ConnectedPeers() []core.PeerID {
	return messenger.Peers()
}

// ConnectedAddresses returns all connected addresses
func (messenger *disabledMessenger) ConnectedAddresses() []string {
	addresses := make([]string, 0)

	return addresses
}

// PeerAddresses returns the virtual peer address
func (messenger *disabledMessenger) PeerAddresses(pid core.PeerID) []string {
	return []string{fmt.Sprintf(virtualAddressTemplate, pid.Pretty())}
}

// ConnectedPeersOnTopic returns the connected peers on the provided topic
func (messenger *disabledMessenger) ConnectedPeersOnTopic(_ string) []core.PeerID {
	return make([]core.PeerID, 0)
}

// SetPeerShardResolver does nothing and returns nil
func (messenger *disabledMessenger) SetPeerShardResolver(_ p2p.PeerShardResolver) error {
	return nil
}

// GetConnectedPeersInfo return current connected peers info
func (messenger *disabledMessenger) GetConnectedPeersInfo() *p2p.ConnectedPeersInfo {
	peersInfo := &p2p.ConnectedPeersInfo{}

	return peersInfo
}

// WaitForConnections does nothing
func (messenger *disabledMessenger) WaitForConnections(_ time.Duration, _ uint32) {
}

// IsConnectedToTheNetwork returns true
func (messenger *disabledMessenger) IsConnectedToTheNetwork() bool {
	return true
}

// ThresholdMinConnectedPeers returns 0
func (messenger *disabledMessenger) ThresholdMinConnectedPeers() int {
	return 0
}

// SetThresholdMinConnectedPeers does nothing and returns nil
func (messenger *disabledMessenger) SetThresholdMinConnectedPeers(_ int) error {
	return nil
}

// SetPeerDenialEvaluator does nothing and returns nil
func (messenger *disabledMessenger) SetPeerDenialEvaluator(_ p2p.PeerDenialEvaluator) error {
	return nil
}

// ID returns the peer ID
func (messenger *disabledMessenger) ID() core.PeerID {
	return messenger.pid
}

// Port returns 0
func (messenger *disabledMessenger) Port() int {
	return 0
}

// Sign will return the hash(messenger.ID + payload)
func (messenger *disabledMessenger) Sign(payload []byte) ([]byte, error) {
	return hasher.Compute(messenger.pid.Pretty() + string(payload)), nil
}

// Verify will check if the provided signature === hash(pid + payload)
func (messenger *disabledMessenger) Verify(payload []byte, pid core.PeerID, signature []byte) error {
	sig := hasher.Compute(pid.Pretty() + string(payload))
	if bytes.Equal(sig, signature) {
		return nil
	}

	return errInvalidSignature
}

// SignUsingPrivateKey will return an empty byte slice
func (messenger *disabledMessenger) SignUsingPrivateKey(_ []byte, _ []byte) ([]byte, error) {
	return make([]byte, 0), nil
}

// SetDebugger will set the provided debugger
func (messenger *disabledMessenger) SetDebugger(_ p2p.Debugger) error {
	return nil
}

// Close does nothing and returns nil
func (messenger *disabledMessenger) Close() error {
	messenger.mutIsClosed.Lock()
	messenger.isClosed = true
	messenger.mutIsClosed.Unlock()

	return nil
}

func (messenger *disabledMessenger) closed() bool {
	messenger.mutIsClosed.RLock()
	defer messenger.mutIsClosed.RUnlock()

	return messenger.isClosed
}

// IsInterfaceNil returns true if there is no value under the interface
func (messenger *disabledMessenger) IsInterfaceNil() bool {
	return messenger == nil
}
