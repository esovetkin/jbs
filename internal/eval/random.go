package eval

import (
	"math/rand"
	"sync"
	"time"
)

type RandomState struct {
	mu  sync.Mutex
	rng *rand.Rand
}

func NewRandomState() *RandomState {
	return NewRandomStateWithSeed(time.Now().UnixNano())
}

func NewRandomStateWithSeed(seed int64) *RandomState {
	return &RandomState{rng: rand.New(rand.NewSource(seed))}
}

func (r *RandomState) SetSeed(seed int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rng.Seed(seed)
}

func (r *RandomState) Intn(n int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rng.Intn(n)
}

func (r *RandomState) Perm(n int) []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rng.Perm(n)
}

var fallbackRandom = NewRandomState()

func randomState(opts ExprOptions) *RandomState {
	if opts.Random != nil {
		return opts.Random
	}
	return fallbackRandom
}
