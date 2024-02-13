package main

import (
	"log"

	"github.com/alistairpialek/airtouch4-go/airtouch"
)

func main() {
	a := airtouch.AirTouch{
		IPAddress:        "x.x.x.x",
		Port:             9004,
		RootTempDir:      "/tmp",
		Timezone:         "Australia/Sydney",
		ReportLoopPeriod: 60,
	}

	err := a.GetGroupData()
	if err != nil {
		log.Panicf("Error getting group data: %s", err)
	}

	log.Printf("Temp of %s is %f", a.Groups[0].Name, a.Groups[0].Temperature)

	for _, g := range a.Groups {
		log.Printf("group.Name = %s", g.Name)
		log.Printf("group.DayDurationMinutes = %f", g.DayDurationMinutes)
	}

	err = a.GetACData()
	if err != nil {
		log.Panicf("Error getting AC data: %s", err)
	}

	log.Printf("AC temp is %f", a.AC.Temperature)

	// After applying fixes, we apply an AC mode switching patch.
	err = a.RunACModeSwitchingPatch()
	if err != nil {
		log.Panicf("Error getting AC data: %s", err)
	}

	err = a.GenerateGroupStatistics()
	if err != nil {
		log.Panicf("Error generating group statistics: %s", err)
	}

	err = a.SetACState("On", "Fan")
	if err != nil {
		log.Panicf("Error setting AC mode: %s", err)
	}

	err = a.SetGroupToTemperature("1", "19")
	if err != nil {
		log.Panicf("Error setting group temperature: %s", err)
	}
}
