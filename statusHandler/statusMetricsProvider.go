package statusHandler

import (
	"strings"
	"sync"

	"github.com/ElrondNetwork/elrond-go/core"
)

// statusMetrics will handle displaying at /node/details all metrics already collected for other status handlers
type statusMetrics struct {
	nodeMetrics *sync.Map
}

// NewStatusMetrics will return an instance of the struct
func NewStatusMetrics() *statusMetrics {
	return &statusMetrics{
		nodeMetrics: &sync.Map{},
	}
}

// IsInterfaceNil returns true if there is no value under the interface
func (sm *statusMetrics) IsInterfaceNil() bool {
	return sm == nil
}

// Increment method increment a metric
func (sm *statusMetrics) Increment(key string) {
	keyValueI, ok := sm.nodeMetrics.Load(key)
	if !ok {
		return
	}

	keyValue, ok := keyValueI.(uint64)
	if !ok {
		return
	}

	keyValue++
	sm.nodeMetrics.Store(key, keyValue)
}

// AddUint64 method increase a metric with a specific value
func (sm *statusMetrics) AddUint64(key string, val uint64) {
	keyValueI, ok := sm.nodeMetrics.Load(key)
	if !ok {
		return
	}

	keyValue, ok := keyValueI.(uint64)
	if !ok {
		return
	}

	keyValue += val
	sm.nodeMetrics.Store(key, keyValue)
}

// Decrement method - decrement a metric
func (sm *statusMetrics) Decrement(key string) {
	keyValueI, ok := sm.nodeMetrics.Load(key)
	if !ok {
		return
	}

	keyValue, ok := keyValueI.(uint64)
	if !ok {
		return
	}
	if keyValue == 0 {
		return
	}

	keyValue--
	sm.nodeMetrics.Store(key, keyValue)
}

// SetInt64Value method - sets an int64 value for a key
func (sm *statusMetrics) SetInt64Value(key string, value int64) {
	sm.nodeMetrics.Store(key, value)
}

// SetUInt64Value method - sets an uint64 value for a key
func (sm *statusMetrics) SetUInt64Value(key string, value uint64) {
	sm.nodeMetrics.Store(key, value)
}

// SetStringValue method - sets a string value for a key
func (sm *statusMetrics) SetStringValue(key string, value string) {
	sm.nodeMetrics.Store(key, value)
}

// Close method - won't do anything
func (sm *statusMetrics) Close() {
}

// StatusMetricsMapWithoutP2P will return the non-p2p metrics in a map
func (sm *statusMetrics) StatusMetricsMapWithoutP2P() map[string]interface{} {
	statusMetricsMap := make(map[string]interface{})
	sm.nodeMetrics.Range(func(key, value interface{}) bool {
		keyString := key.(string)
		if strings.Contains(keyString, "_p2p_") {
			return true
		}

		statusMetricsMap[key.(string)] = value
		return true
	})

	return statusMetricsMap
}

// StatusP2pMetricsMap will return the p2p metrics in a map
func (sm *statusMetrics) StatusP2pMetricsMap() map[string]interface{} {
	statusMetricsMap := make(map[string]interface{})
	sm.nodeMetrics.Range(func(key, value interface{}) bool {
		keyString := key.(string)
		if !strings.Contains(keyString, "_p2p_") {
			return true
		}

		statusMetricsMap[key.(string)] = value
		return true
	})

	return statusMetricsMap
}

// EpochMetrics will return metrics related to current epoch
func (sm *statusMetrics) EpochMetrics() map[string]interface{} {
	epochMetrics := make(map[string]interface{})
	var currentEpoch uint32
	var roundNumberAtEpochStart uint64
	var roundsPerEpoch uint32
	var currentRound uint64
	var ok bool
	currentEpochObj, ok := sm.nodeMetrics.Load(core.MetricEpochNumber)
	if ok {
		currentEpoch = uint32(currentEpochObj.(uint64))
	}
	roundNumberAtEpochStartObj, ok := sm.nodeMetrics.Load(core.MetricRoundAtEpochStart)
	if ok {
		roundNumberAtEpochStart = roundNumberAtEpochStartObj.(uint64)
	}
	roundsPerEpochObj, ok := sm.nodeMetrics.Load(core.MetricRoundsPerEpoch)
	if ok {
		roundsPerEpoch = uint32(roundsPerEpochObj.(uint64))
	}
	currentRoundObj, ok := sm.nodeMetrics.Load(core.MetricCurrentRound)
	if ok {
		currentRound = currentRoundObj.(uint64)
	}

	epochMetrics[core.MetricEpochNumber] = currentEpoch
	epochMetrics[core.MetricRoundAtEpochStart] = roundNumberAtEpochStart
	epochMetrics[core.MetricRoundsPerEpoch] = roundsPerEpoch
	epochMetrics[core.MetricCurrentRound] = currentRound
	epochMetrics[core.MetricRoundsPassedInCurrentEpoch] = currentRound - roundNumberAtEpochStart

	return epochMetrics
}