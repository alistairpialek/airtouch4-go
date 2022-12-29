package airtouch

import (
	"fmt"
	"log"
	"strconv"

	"github.com/elliotchance/orderedmap"
)

// AC models AC attributes.
type AC struct {
	AcMode           string
	AcTargetSetpoint int
	Temperature      float64
	Spill            bool
}

// ACModeMap maps stringy AC modes to their numerical value.
func (a *AirTouch) ACModeMap() map[string]string {
	m := make(map[string]string)

	m["Auto"] = "0"
	m["Heat"] = "1"
	m["Dry"] = "2"
	m["Fan"] = "3"
	m["Cool"] = "4"
	m["AutoHeat"] = "8"
	m["AutoCool"] = "9"

	return m
}

// ACStatusMap is used to find the corresponding attribute from the reply message.
func (a *AirTouch) ACStatusMap() map[string]string {
	m := make(map[string]string)

	m["PowerState"] = "1:7-8"
	m["AcNumber"] = "1:1-6"
	m["AcMode"] = "2:5-8"
	m["AcFanSpeed"] = "2:1-4"
	m["Spill"] = "3:8-8"
	m["AcTimer"] = "3:7-7"
	m["AcTargetSetpoint"] = "3:1-6"
	m["Temperature"] = "5:6-16"

	return m
}

// ACControlMap sets default values.
func (a *AirTouch) ACControlMap() *orderedmap.OrderedMap {
	m := orderedmap.NewOrderedMap()

	m.Set("Power", "1:7-8")
	m.Set("AcNumber", "1:1-6")
	m.Set("AcMode", "2:5-8")
	m.Set("AcFanSpeed", "2:1-4")
	m.Set("SetpointControlType", "3:7-8")
	m.Set("TargetSetpoint", "3:1-6")
	m.Set("ZeroedByte", "4:1-8")

	return m
}

// GetACData retrieves AC data and sends to configured outputs.
func (a *AirTouch) GetACData() error {
	err := a.GetACStatus()
	if err != nil {
		return err
	}

	log.Printf("AC Temperature: %.1f", a.AC.Temperature)
	log.Printf("AC TargetSetpoint: %d", a.AC.AcTargetSetpoint)

	return nil
}

// GetACStatus sends and decodes the ACStatus reply.
func (a *AirTouch) GetACStatus() error {
	messageIn := MessageInput{
		Message: ACStatus,
	}

	messageOut, err := a.CommunicateMessage(&messageIn)
	if err != nil {
		return err
	}

	err = a.DecodeACStatusMessage(*messageOut)
	if err != nil {
		return err
	}

	return nil
}

// SetCoolingModeForAC adjusts the ACControlMap to set the desired AC operating mode.
func (a *AirTouch) SetCoolingModeForAC(acMode string) error {
	controlMessage := a.ACControlMap()
	controlMessage.Set("Power", "0")
	controlMessage.Set("AcNumber", "0")
	controlMessage.Set("AcMode", "0")
	controlMessage.Set("AcFanSpeed", "0")
	controlMessage.Set("SetpointControlType", "0")
	controlMessage.Set("TargetSetpoint", "0")
	controlMessage.Set("ZeroedByte", "0")

	// These are required to leave these settings unchanged.
	controlMessage.Set("AcMode", a.ACModeMap()[acMode])
	controlMessage.Set("AcFanSpeed", "15")
	controlMessage.Set("TargetSetpoint", "63")
	controlMessage.Set("AcNumber", "0")

	message, err := a.MessageObjectToMessagePacket(controlMessage)
	if err != nil {
		return err
	}

	//a.Log.Debug("sending message = %s", message)

	messageIn := MessageInput{
		Message: *message,
	}

	messageOut, err := a.CommunicateMessage(&messageIn)
	if err != nil {
		return err
	}

	err = a.DecodeACStatusMessage(*messageOut)
	if err != nil {
		return err
	}

	return nil
}

// MessageObjectToMessagePacket transforms our object to a string we can then send to the AC.
func (a *AirTouch) MessageObjectToMessagePacket(messageObject *orderedmap.OrderedMap) (*string, error) {

	messageString := "80b001" + ACControl

	acControlPacketLocationMap := a.ACControlMap()

	binaryMessagePayloadString := ""

	for _, k := range acControlPacketLocationMap.Keys() {
		//a.Log.Debug("Value is = %s", l)
		//a.Log.Debug("Key is = %s", k)
		value, _ := acControlPacketLocationMap.Get(k)
		valueAsString := value.(string)

		messageValue, _ := messageObject.Get(k)
		messageValueAsString := messageValue.(string)

		binValue, err := a.AddMapValueToBinaryValue(binaryMessagePayloadString, valueAsString, messageValueAsString)
		if err != nil {
			return nil, err
		}
		binaryMessagePayloadString = *binValue
	}

	//a.Log.Debug("dataPayloadBinary: %s", binaryMessagePayloadString)

	binaryMessagePayloadBase2, err := strconv.ParseInt(binaryMessagePayloadString, 2, 64)
	if err != nil {
		return nil, err
	}
	//a.Log.Debug("binaryMessagePayloadBase2: %d", binaryMessagePayloadBase2)

	dataPayload := fmt.Sprintf("%08x", binaryMessagePayloadBase2)
	//a.Log.Debug("binaryMessagePayloadHex: %s", dataPayload)

	dataLength := len(dataPayload) / 2
	lenA := len(fmt.Sprintf("%x", dataLength))
	//a.Log.Debug("lenA: %d", lenA)
	lenB := fmt.Sprintf("%x", dataLength)
	//a.Log.Debug("lenB: %s", lenB)
	lengthString := "0000"[0:4-(lenA)] + lenB
	//a.Log.Debug("lengthString: %s", lengthString)

	messageString += lengthString + dataPayload
	//a.Log.Debug("messageString: %s", messageString)

	return &messageString, nil

	// messageIn := MessageInput{
	// 	Message: SetCoolingModeForAC,
	// }
}
