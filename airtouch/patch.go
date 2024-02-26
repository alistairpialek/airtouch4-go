package airtouch

import (
	"errors"
	"log"
)

type groupTemperature struct {
	name             string
	diffSetpointTemp float64
	currentTemp      float64
}

// RunACModeSwitchingPatch Currently, when cooling, the spill will eventually become active after satisfying the currently selected
// cooling algorithm e.g. average. For some reason, it takes a really long time for the AC the stop the compressor...
// During this time, the spill group/room is freezing and copious amounts of power is used unnecessarily cooling.
// 1) What this patch does is detect when the AC mode is cooling AND the spill is active.
// 2) Switch the AC mode to Fresh.
// 3) When SensorTemp - FreshSetpointTemp >= 1 AND (Nursery is On AND Mode == Percentage AND OpenPercentage == 95%), switch AC mode to Cooling
// The additional conditional on 3) allows the user to opt-out of this patch if they genuinely want to run Fresh without
// it switching back to cooling mode automatically.
func (a *AirTouch) RunACModeSwitchingPatch() error {
	log.Printf("AC mode is currently = %s", a.AC.AcMode)
	if !(a.AC.AcMode == "Cool" || a.AC.AcMode == "Heat" || a.AC.AcMode == "Fan") {
		log.Printf("Unsupported AC mode %s, skipping running patch", a.AC.AcMode)
		return nil
	}

	// Record the AC mode so that when we switch back to Fan, we know whether we are meant to be Heating or Cooling.
	if a.AC.AcMode == "Cool" || a.AC.AcMode == "Heat" {
		err := a.WriteValueToFile("current_ac_mode", a.AC.AcMode)
		if err != nil {
			log.Printf("Unable to write mode to file, please correct, skipping running patch")
			return nil
		}
	}

	// Fan mode does not have a setpoint in the app. But we define one here so that we know at what point we need to
	// switch the AC mode back to Cooling.
	acBackToCoolingToleranceTemp := 0.3
	acBackToHeatingToleranceTemp := -0.3
	//acBackToFanSpillTolerancePct := 70

	//a.Log.Debug("Fan tolerance temp from conf is = %f", acBackToCoolingToleranceTemp)

	//groupSpill := false
	//groupSpillOpenPercentage := -1

	// for _, g := range a.Groups {
	// 	if g.Spill {
	// 		groupSpill = true
	// 		groupSpillOpenPercentage = g.SpillPercentage
	// 	}
	// }

	// Need to know whether we are heating or cooling as to whether we are finding the coldest or warmest room.
	focusGroup, err := a.getTemperature()
	if err != nil {
		log.Printf("Unable to determine if we're meant to be heating or cooling, try setting a mode?")
		return nil
	}

	log.Printf("%s has the biggest temp difference %f", focusGroup.name, focusGroup.diffSetpointTemp)
	log.Printf("Using this as the AC temp")
	acTemperature := focusGroup.currentTemp

	lastACMode, err := a.ReadStringFromFile("current_ac_mode")
	if err != nil {
		log.Printf("Unable to determine if we're meant to be heating or cooling, try setting a mode?")
		return nil
	}

	// Wait until we are meaningfully spilling before switching back to Fan.
	if a.AC.AcMode != "Fan" {
		// 	if lastACMode == "Cool" {

		// 	}

		// && groupSpill && groupSpillOpenPercentage >= acBackToFanSpillTolerancePct {
		// 	a.Log.Debug("AC mode is %s and spill is active, setting AC mode to Fan", a.AC.AcMode)

		// 	err := a.SetCoolingModeForAC("Fan")
		// 	if err != nil {
		// 		return err
		// 	}

		if lastACMode == "Cool" {
			log.Printf("Tolerance is %f", acBackToCoolingToleranceTemp)

			// At temperature or cooler.
			if focusGroup.diffSetpointTemp <= 0 {
				log.Printf("Group temp diff %f is less than 0, so turning Fan mode on", focusGroup.diffSetpointTemp)
				err := a.SetACState("On", "Fan")
				if err != nil {
					return err
				}
			} else {
				log.Printf("Group temp diff %f is greater than 0, so keeping Cool mode on", focusGroup.diffSetpointTemp)
				//a.Log.Debug("Temp condition to turn AC back to Fan NOT satisfied")

			}
		} else if lastACMode == "Heat" {
			log.Printf("Tolerance is %f", acBackToHeatingToleranceTemp)

			// At temperature or warmer.
			if focusGroup.diffSetpointTemp >= 0 {
				log.Printf("Group temp diff %f is greater than 0, so turning Fan mode on", focusGroup.diffSetpointTemp)
				err := a.SetACState("On", "Fan")
				if err != nil {
					return err
				}
			} else {
				log.Printf("Group temp diff %f is less than 0, so keeping Heat mode on", focusGroup.diffSetpointTemp)
				//a.Log.Debug("Temp condition to turn AC back to Fan NOT satisfied")
			}
		}
	} else if a.AC.AcMode == "Fan" {
		log.Printf("AC mode is Fan and Temp is %f", acTemperature)

		//currentTempDiff := acTemperature - float64(a.AC.AcTargetSetpoint)
		log.Printf("Temp difference between group current and setpoint is = %f", focusGroup.diffSetpointTemp)

		// a.Log.Debug("Total groups open is %d", groupsOn)

		if lastACMode == "Cool" {
			log.Printf("Tolerance is %f", acBackToCoolingToleranceTemp)

			if focusGroup.diffSetpointTemp >= acBackToCoolingToleranceTemp {
				log.Printf("Temp condition to turn AC back to Cool satisfied")

				err := a.SetACState("On", "Cool")
				if err != nil {
					return err
				}
			} else {
				log.Printf("Group temp diff %f is less than tolerance %f, so keeping Fan mode on", focusGroup.diffSetpointTemp, acBackToCoolingToleranceTemp)
			}
		} else if lastACMode == "Heat" {
			log.Printf("Tolerance is %f", acBackToHeatingToleranceTemp)

			if focusGroup.diffSetpointTemp <= acBackToHeatingToleranceTemp {
				log.Printf("Temp condition to turn AC back to Heat satisfied")

				err := a.SetACState("On", "Heat")
				if err != nil {
					return err
				}
			} else {
				log.Printf("Group temp diff %f is greater than tolerance %f, so keeping Fan mode on", focusGroup.diffSetpointTemp, acBackToHeatingToleranceTemp)
			}
		}
	}

	return nil
}

func (a *AirTouch) getTemperature() (*groupTemperature, error) {
	var focusGroup groupTemperature
	var acMode string
	var err error

	// Init temp values
	if a.AC.AcMode == "Cool" {
		focusGroup.diffSetpointTemp = -50.0
		acMode = "Cool"
	} else if a.AC.AcMode == "Heat" {
		focusGroup.diffSetpointTemp = 50.0
		acMode = "Heat"
	} else if a.AC.AcMode == "Fan" {
		// We're on Fan now, but dig out what we were using previously.
		acMode, err = a.ReadStringFromFile("current_ac_mode")
		if err != nil {
			return nil, errors.New("unable to determine if we're meant to be heating or cooling, try setting a mode?")
		}

		if acMode == "Cool" {
			focusGroup.diffSetpointTemp = -50.0
		} else if acMode == "Heat" {
			focusGroup.diffSetpointTemp = 50.0
		}
	}

	focusGroup.name = "NA"
	focusGroup.currentTemp = 0.0

	for _, g := range a.Groups {

		// 22.2 - 23 = -0.8 under setpoint
		// -50 < -0.8 (true), -0.8 < -0.6 (bit warmer, becomes new)
		// 23 - 23 = 0
		// -50 < -0.8 (true)
		// -0.8 < 0 (true)
		// 19.0 - 20.0 = -1, 50 > -1
		tempDiffSetpointTemp := g.Temperature - float64(g.TargetSetpoint)

		if g.PowerState == "On" && acMode == "Cool" && (focusGroup.diffSetpointTemp < tempDiffSetpointTemp) {
			focusGroup.diffSetpointTemp = tempDiffSetpointTemp
			focusGroup.name = g.Name
			focusGroup.currentTemp = g.Temperature
		} else if g.PowerState == "On" && acMode == "Heat" && (focusGroup.diffSetpointTemp > tempDiffSetpointTemp) {
			focusGroup.diffSetpointTemp = tempDiffSetpointTemp
			focusGroup.name = g.Name
			focusGroup.currentTemp = g.Temperature
		}
	}

	return &focusGroup, nil
}

func (a *AirTouch) EscapeProgramming() bool {
	for _, g := range a.Groups {
		if g.Name == "Nursery" && g.PowerState == "On" && g.ControlMethod == "PercentageControl" && g.OpenPercentage == 95 {
			log.Printf("Criteria to skip programming met, keeping Fan on")
			return true
		}
	}
	return false
}
