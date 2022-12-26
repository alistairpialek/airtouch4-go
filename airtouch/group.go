package airtouch

import (
	"log"
)

// Group models group attributes.
type Group struct {
	// PowerState is either On or Off.
	PowerState         string
	Name               string
	Number             int
	ControlMethod      string
	OpenPercentage     int
	BatteryLow         bool
	TurboSupport       bool
	TargetSetpoint     int
	Sensor             bool
	Temperature        float64
	Spill              bool
	SpillPercentage    int
	DayDurationMinutes float64
}

// GroupStatusMap is used to find the corresponding attribute from the reply message.
func (a *AirTouch) GroupStatusMap() map[string]string {
	m := make(map[string]string)

	m["PowerState"] = "1:7-8"
	m["GroupNumber"] = "1:1-6"

	m["ControlMethod"] = "2:8-8"
	m["OpenPercentage"] = "2:1-7"
	m["BatteryLow"] = "3:8-8"
	m["TurboSupport"] = "3:7-7"
	m["TargetSetpoint"] = "3:1-6"
	m["Sensor"] = "4:8-8"
	m["Temperature"] = "5:6-16"
	m["Spill"] = "6:5-5"

	return m
}

// GetGroupData retrieves group data and sends to configured outputs.
func (a *AirTouch) GetGroupData() error {
	// Group status needs to go first so that AC groups are created.
	err := a.GetGroupStatus()
	if err != nil {
		return err
	}

	// Add the group names and numbers to the AC groups.
	err = a.GetGroupName()
	if err != nil {
		return err
	}

	// Groups that are off still report their last OpenPercentage :(
	// OpenPercentage for spilling vents also needs to be corrected.
	a.FixOpenPercentages()

	for _, group := range a.Groups {
		log.Printf("Name: %s", group.Name)
		log.Printf("Number: %d", group.Number)
		log.Printf("PowerState: %s", group.PowerState)
		log.Printf("Temperature: %.1f", group.Temperature)
		log.Printf("ControlMethod: %s", group.ControlMethod)
		log.Printf("TargetSetpoint: %d", group.TargetSetpoint)
		log.Printf("OpenPercentage: %d", group.OpenPercentage)
		log.Printf("Spill: %t", group.Spill)
		log.Printf("SpillPct: %d", group.SpillPercentage)
		log.Printf("--------------------------------------------------")
	}

	return nil
}

// GetGroupName sends a message to get group names.
func (a *AirTouch) GetGroupName() error {
	messageIn := MessageInput{
		Message: GroupName,
	}

	messageOut, err := a.CommunicateMessage(&messageIn)
	if err != nil {
		return err
	}

	err = a.DecodeGroupNameMessage(*messageOut)
	if err != nil {
		return err
	}

	return nil
}

// GetGroupStatus sends a message to get group status attributes.
func (a *AirTouch) GetGroupStatus() error {
	messageIn := MessageInput{
		Message: GroupStatus,
	}

	messageOut, err := a.CommunicateMessage(&messageIn)
	if err != nil {
		return err
	}

	err = a.DecodeGroupStatusMessage(*messageOut)
	if err != nil {
		return err
	}

	return nil
}
