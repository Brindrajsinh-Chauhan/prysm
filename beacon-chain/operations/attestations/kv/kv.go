// Package kv includes a key-value store implementation
// of an attestation cache used to satisfy important use-cases
// such as aggregation in a beacon node runtime.
package kv

import (
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/crypto/hash"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

var hashFn = hash.Proto

// AttCaches defines the caches used to satisfy attestation pool interface.
// These caches are KV store for various attestations
// such are unaggregated, aggregated or attestations within a block.
type AttCaches struct {
	aggregatedAttLock  sync.RWMutex
	aggregatedAtt      map[[32]byte][]ethpb.Att
	unAggregateAttLock sync.RWMutex
	unAggregatedAtt    map[[32]byte]ethpb.Att
	forkchoiceAttLock  sync.RWMutex
	forkchoiceAtt      map[[32]byte]ethpb.Att
	blockAttLock       sync.RWMutex
	blockAtt           map[[32]byte][]ethpb.Att
	seenAtt            *cache.Cache
}

// NewAttCaches initializes a new attestation pool consists of multiple KV store in cache for
// various kind of attestations.
func NewAttCaches() *AttCaches {
	secsInEpoch := time.Duration(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
	c := cache.New(secsInEpoch*time.Second, 2*secsInEpoch*time.Second)
	pool := &AttCaches{
		unAggregatedAtt: make(map[[32]byte]ethpb.Att),
		aggregatedAtt:   make(map[[32]byte][]ethpb.Att),
		forkchoiceAtt:   make(map[[32]byte]ethpb.Att),
		blockAtt:        make(map[[32]byte][]ethpb.Att),
		seenAtt:         c,
	}

	return pool
}
