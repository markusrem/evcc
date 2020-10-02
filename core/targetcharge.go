package core

import "time"

const (
	utilization float64 = 0.6
	deviation           = 30 * time.Minute
)

// TargetCharge is the target charging handler
type TargetCharge struct {
	*LoadPoint
	SoC            int
	Time           time.Time
	finishAt       time.Time
	chargeRequired bool
}

// Supported returns true if target charging is possible, i.e. the vehicle soc can be determined
func (lp TargetCharge) Supported() bool {
	return lp.socEstimator != nil
}

// Active returns true if there is an active target charging request
func (lp TargetCharge) Active() bool {
	inactive := lp.Time.IsZero() || lp.Time.Before(time.Now())
	lp.publish("socTimerSet", !inactive)

	// reset active
	if inactive && lp.chargeRequired {
		lp.chargeRequired = false
		lp.publish("socTimerActive", lp.chargeRequired)
	}

	return !inactive
}

// Reset resets the target charging request
func (lp TargetCharge) Reset() {
	lp.Time = time.Time{}
	lp.SoC = 0
}

// StartRequired calculates remaining charge duration and returns true if charge start is required to achieve target soc in time
func (lp TargetCharge) StartRequired() bool {
	current := lp.effectiveCurrent()

	// use start current for calculation if currently not charging
	if current == 0 {
		current = int64(float64(lp.MaxCurrent) * utilization)
		current = clamp(current, lp.MinCurrent, lp.MaxCurrent)
	}

	power := float64(current*lp.Phases) * Voltage

	// time
	remainingDuration := lp.socEstimator.RemainingChargeDuration(power, lp.SoC)
	lp.finishAt = time.Now().Add(remainingDuration).Round(time.Minute)

	lp.log.DEBUG.Printf("target charging remaining time: %v (finish %v at %.1f utilization)", remainingDuration.Round(time.Minute), lp.finishAt, utilization)

	lp.chargeRequired = lp.finishAt.After(lp.Time)
	lp.publish("socTimerActive", lp.chargeRequired)

	return lp.chargeRequired
}

// Handle adjusts current up/down to achieve desired target time
func (lp TargetCharge) Handle() error {
	current := lp.handler.TargetCurrent()

	switch {
	case lp.finishAt.Before(lp.Time.Add(-deviation)):
		current--
		lp.log.DEBUG.Printf("target charging: slowdown")

	case lp.finishAt.After(lp.Time):
		current++
		lp.log.DEBUG.Printf("target charging: speedup")
	}

	current = clamp(current, lp.MinCurrent, lp.MaxCurrent)

	return lp.handler.Ramp(current)
}