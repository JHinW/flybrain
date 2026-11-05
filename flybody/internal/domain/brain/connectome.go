package brain

import (
	"math"
	"sort"
	"strings"

	"github.com/jhinw/flybrain/flybody/internal/domain/organ"
)

// ConceptConnectome belongs to the Domain Layer.
// Primary Responsibility: Execute double-buffered synaptic signal propagation across the
// 59 Drosophila functional neuron groups, cascading supra-threshold neurons and draining
// efferent motor neuron readouts.
type ConceptConnectome struct {
	weights       map[string]map[string]float64
	neuronOrder   []string
	postSynaptic  map[string][2]float64
	thisState     int
	nextState     int
	fireThreshold float64
	lastFired     []string
	lastMotor     organ.MotorReadout
}

// NewConceptConnectome constructs and initializes the 59-group Drosophila functional connectome.
func NewConceptConnectome() *ConceptConnectome {
	w := DefaultWeights()
	allSet := make(map[string]bool)
	for pre, targets := range w {
		allSet[pre] = true
		for post := range targets {
			allSet[post] = true
		}
	}
	order := make([]string, 0, len(allSet))
	for name := range allSet {
		order = append(order, name)
	}
	sort.Strings(order)

	post := make(map[string][2]float64, len(order))
	for _, name := range order {
		post[name] = [2]float64{0, 0}
	}

	c := &ConceptConnectome{
		weights:       w,
		neuronOrder:   order,
		postSynaptic:  post,
		thisState:     0,
		nextState:     1,
		fireThreshold: 22.0,
	}
	// Warm up baseline central complex reverberation (matching tonic excitation in connectome.js)
	c.seedBaseline()
	return c
}

func (c *ConceptConnectome) seedBaseline() {
	for _, target := range []string{"CX_FC", "CX_EPG", "CX_PFN"} {
		c.addPotential(target, 12.0)
	}
}

func (c *ConceptConnectome) addPotential(neuron string, delta float64) {
	buf, ok := c.postSynaptic[neuron]
	if !ok {
		return
	}
	buf[c.nextState] += delta
	c.postSynaptic[neuron] = buf
}

// dendriteAccumulateScaled projects weighted synaptic currents from preSynaptic to all its postSynaptic targets.
func (c *ConceptConnectome) dendriteAccumulateScaled(preSynaptic string, scale float64) {
	targets, ok := c.weights[preSynaptic]
	if !ok {
		return
	}
	for post, weight := range targets {
		c.addPotential(post, math.Round(weight*scale))
	}
}

// Propagate runs one full synaptic tick:
// 1. Injects afferent sensory and drive spikes via dendritic accumulation;
// 2. Injects tonic Central Complex background activity (CX_FC, CX_EPG, CX_PFN);
// 3. Fires all non-motor neurons whose membrane potential exceeds fireThreshold (22);
// 4. Drains motor neurons (MN_*) into a MotorReadout and swaps double buffers.
func (c *ConceptConnectome) Propagate(spikes []organ.AfferentSpike, ambientLight float64) organ.MotorReadout {
	firedSet := make(map[string]bool)

	// 1. Stimulate sensory and drive neurons from afferent spikes
	for _, sp := range spikes {
		pulses := sp.Pulses
		if pulses <= 0 {
			pulses = 1
		}
		scale := sp.Scale
		if scale <= 0 {
			scale = 1.0
		}
		for p := 0; p < pulses; p++ {
			c.dendriteAccumulateScaled(sp.TargetGroup, scale)
		}
		firedSet[sp.TargetGroup] = true
	}

	// 2. Tonic background activity in Central Complex (matching connectome.js lines 377-383)
	tonicLevel := 8.0
	if ambientLight <= 0.05 {
		tonicLevel = 4.0
	}
	for _, tonicTarget := range []string{"CX_FC", "CX_EPG", "CX_PFN"} {
		c.addPotential(tonicTarget, tonicLevel)
	}

	// 3. Fire neurons whose current buffer (thisState) exceeds fireThreshold (22.0)
	for _, ps := range c.neuronOrder {
		if isMotorNeuron(ps) {
			continue
		}
		if c.postSynaptic[ps][c.thisState] > c.fireThreshold {
			c.fireNeuron(ps)
			firedSet[ps] = true
		}
	}

	// 4. Drain motor neurons into MotorReadout
	readout := c.drainMotorControl()
	c.lastMotor = readout

	// 5. Copy nextState into thisState, then swap buffer indices (matching connectome.js lines 413-420)
	for _, ps := range c.neuronOrder {
		buf := c.postSynaptic[ps]
		buf[c.thisState] = buf[c.nextState]
		c.postSynaptic[ps] = buf
	}
	c.thisState, c.nextState = c.nextState, c.thisState

	firedList := make([]string, 0, len(firedSet))
	for name := range firedSet {
		firedList = append(firedList, name)
	}
	sort.Strings(firedList)
	c.lastFired = firedList

	return readout
}

func (c *ConceptConnectome) fireNeuron(neuron string) {
	c.dendriteAccumulateScaled(neuron, 1.0)
	buf := c.postSynaptic[neuron]
	buf[c.nextState] = 0
	c.postSynaptic[neuron] = buf
}

func (c *ConceptConnectome) readAndDrainMotor(name string) float64 {
	buf, ok := c.postSynaptic[name]
	if !ok {
		return 0
	}
	val := buf[c.nextState]
	buf[c.nextState] = 0
	c.postSynaptic[name] = buf
	if val < 0 {
		return 0
	}
	return val
}

func (c *ConceptConnectome) drainMotorControl() organ.MotorReadout {
	legL1 := c.readAndDrainMotor("MN_LEG_L1")
	legR1 := c.readAndDrainMotor("MN_LEG_R1")
	legL2 := c.readAndDrainMotor("MN_LEG_L2")
	legR2 := c.readAndDrainMotor("MN_LEG_R2")
	legL3 := c.readAndDrainMotor("MN_LEG_L3")
	legR3 := c.readAndDrainMotor("MN_LEG_R3")
	wingL := c.readAndDrainMotor("MN_WING_L")
	wingR := c.readAndDrainMotor("MN_WING_R")
	proboscis := c.readAndDrainMotor("MN_PROBOSCIS")
	head := c.readAndDrainMotor("MN_HEAD")
	abdomen := c.readAndDrainMotor("MN_ABDOMEN")

	startle := 0.0
	if buf, ok := c.postSynaptic["DN_STARTLE"]; ok {
		if buf[c.nextState] > 0 {
			startle = buf[c.nextState]
		}
	}

	return organ.MotorReadout{
		LegL1:     legL1,
		LegR1:     legR1,
		LegL2:     legL2,
		LegR2:     legR2,
		LegL3:     legL3,
		LegR3:     legR3,
		WingL:     wingL,
		WingR:     wingR,
		Proboscis: proboscis,
		Head:      head,
		Abdomen:   abdomen,
		Startle:   startle,
	}
}

func isMotorNeuron(name string) bool {
	return strings.HasPrefix(name, "MN_")
}

// ActiveGroups returns the sorted list of neuron groups that fired or were stimulated in the last tick.
func (c *ConceptConnectome) ActiveGroups() []string {
	out := make([]string, len(c.lastFired))
	copy(out, c.lastFired)
	return out
}

// TopPotentials returns the highest membrane potentials currently buffered in the brain.
func (c *ConceptConnectome) TopPotentials(limit int) []NeuronPotential {
	firedMap := make(map[string]bool, len(c.lastFired))
	for _, g := range c.lastFired {
		firedMap[g] = true
	}
	list := make([]NeuronPotential, 0, len(c.neuronOrder))
	for _, name := range c.neuronOrder {
		v := c.postSynaptic[name][c.thisState]
		if v != 0 || firedMap[name] {
			list = append(list, NeuronPotential{
				Group:     name,
				Potential: v,
				Fired:     firedMap[name],
			})
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Potential == list[j].Potential {
			return list[i].Group < list[j].Group
		}
		return list[i].Potential > list[j].Potential
	})
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list
}

// Snapshot exports the complete brain state for persistence in .flysoul.json.
func (c *ConceptConnectome) Snapshot() BrainSnapshot {
	pots := make(map[string]float64, len(c.neuronOrder))
	for _, name := range c.neuronOrder {
		pots[name] = c.postSynaptic[name][c.thisState]
	}
	firedCopy := make([]string, len(c.lastFired))
	copy(firedCopy, c.lastFired)
	return BrainSnapshot{
		EngineType:       "ConceptConnectome-59",
		FireThreshold:    c.fireThreshold,
		Potentials:       pots,
		LastFiredGroups:  firedCopy,
		LastMotorReadout: c.lastMotor,
	}
}

// Restore loads membrane potentials and firing history from a BrainSnapshot.
func (c *ConceptConnectome) Restore(snap BrainSnapshot) {
	if snap.FireThreshold > 0 {
		c.fireThreshold = snap.FireThreshold
	}
	for _, name := range c.neuronOrder {
		val := snap.Potentials[name]
		c.postSynaptic[name] = [2]float64{val, val}
	}
	c.lastFired = append([]string(nil), snap.LastFiredGroups...)
	c.lastMotor = snap.LastMotorReadout
}
