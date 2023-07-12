package airtouch

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/npat-efault/crc16"
)

// MessageInput models the message to send to the Airtouch 4 console.
type MessageInput struct {
	Message        string
	MessageWithCRC string
}

const (
	// GroupStatus is used to query group status attributes.
	GroupStatus = "80b0012b0000"
	// GroupName is used to query the group names.
	GroupName = "90b0011f0002ff12"
	// ACStatus is used to query the AC status attributes.
	ACStatus = "80b0012d0000f4cf"
	// ACControl is used to send messages to the AC.
	ACControl = "2c"
	// GroupControl is used to send messages to the AC to control groups.
	GroupControl = "2a"
)

// MessageOutput models the Airtouch 4 reply message.
type MessageOutput struct {
	Address []byte
	ID      []byte
	Type    []byte
	Length  []byte
	Body    []byte
}

// PrepareMessage takes a message, performs a checksum and hex encodes a message with CRC.
func (a *AirTouch) PrepareMessage(message *MessageInput) error {
	data, err := hex.DecodeString(message.Message)
	if err != nil {
		return err
	}
	//a.Log.Info("fromHex = % x", data)

	checksum := crc16.Checksum(&crc16.Conf{
		Poly: 0x8005, BitRev: true,
		IniVal: 0xffff, FinVal: 0x0,
		BigEnd: false,
	}, data)
	//a.Log.Info("checksum = %d", checksum)

	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, checksum)
	//fmt.Println(string(b))
	//a.Log.Info("b = %v", b)

	encoded := hex.EncodeToString(b)
	//a.Log.Info("encoded = %s", encoded)

	// padded := fmt.Sprintf("%08x", encoded)
	// a.Log.Info("padded = %s", padded)
	// toCRC16 = 62767

	message.MessageWithCRC = fmt.Sprintf("5555%s%s", message.Message, encoded)
	//a.Log.Info("message = %s", messageString)

	// formatted = 0001`	0f52f
	// chopped = f52f
	// 555580b0012b0000f52f

	// groupStatus: 5555 80b0012b0000 f52f
	// groupName: 5555 90b0011f0002ff12 820c
	// acStatus: 5555 80b0012d0000f4cf e352
	// acAbility: 5555 90b0011f0002ff11 834c

	// + format(crc16(bytes.fromhex(messageString)), '08x')[4:]

	return nil
}

// SendMessage connects to the Airtouch 4 console and sends a message to retrieve
// AC or group info.
func (a *AirTouch) SendMessage(message *string) ([]byte, error) {
	hostname := net.ParseIP(a.IPAddress)
	port := a.Port
	bufferSize := 1024

	// Create TCP address.
	tcpAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", hostname, port))
	if err != nil {
		return nil, fmt.Errorf("tcpAddr: %s", err)
	}

	// Make connection.
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return nil, fmt.Errorf("dialtimeout: %s", err)
	}

	// Set timeout.
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	messageToSend := make([]byte, hex.DecodedLen(len([]byte(*message))))
	hex.Decode(messageToSend, []byte(*message))

	// Send message.
	//a.Log.Debug("messageString: %s", *message)
	//a.Log.Debug("messageToSend: %v", messageToSend)

	_, err = conn.Write(messageToSend)
	if err != nil {
		return nil, fmt.Errorf("connwrite: %s", err)
	}

	//a.Log.Debug("wrote: %d", written)

	reply := make([]byte, bufferSize)
	_, err = conn.Read(reply)
	if err != nil {
		return nil, fmt.Errorf("reading reply: %s", err)
	}

	//a.Log.Debug("read: %d", read)
	//a.Log.Debug("reply: %v", reply)

	return reply, nil
}

// TranslatePacketToMessage decodes the server reply.
func (a *AirTouch) TranslatePacketToMessage(dataResult []byte) (MessageOutput, error) {
	//a.Log.Debug("starting with: %v", dataResult)

	response := MessageOutput{
		Address: dataResult[2:4],
		ID:      dataResult[4:5],
		Type:    dataResult[5:6],
		Length:  dataResult[6:8],
		Body:    dataResult[8:],
	}

	//a.Log.Debug("address: %v", response.Address)
	//a.Log.Debug("messageId: %v", response.Id)
	//a.Log.Debug("messageType: %x", response.Type)
	//a.Log.Debug("dataLength: %v", response.Length)
	//a.Log.Debug("body: %v", response.Body)

	return response, nil
}

// DecodeGroupNameMessage decodes the group name which is not returned with the status request.
func (a *AirTouch) DecodeGroupNameMessage(response MessageOutput) error {
	//a.Log.Debug("groupname: %v", response.Body)

	for i, chunk := range chunk(response.Body[2:], 9) {
		if i > 3 {
			break
		}

		groupNumber := chunk[0]
		groupName := chunk[1:9]
		//a.Log.Debug("groupNumber: %d", groupNumber)
		//a.Log.Debug("groupName: %s", groupName)
		// Remove any NULL characters
		a.Groups[groupNumber].Name = string(bytes.Trim(groupName, "\x00"))
	}

	return nil
}

// DecodeACStatusMessage decodes the AC status. An AC has many groups. The AC itself
// has many attributes.
func (a *AirTouch) DecodeACStatusMessage(response MessageOutput) error {
	packetInfoLocationMap := a.ACStatusMap()

	for i, chunk := range chunk(response.Body, 8) {
		if i > 0 {
			break
		}

		for k := range packetInfoLocationMap {

			mapValue, err := a.TranslateMapValueToValue(chunk, packetInfoLocationMap[k])
			if err != nil {
				return err
			}

			// a.Log.Debug("key: %s", k)
			// a.Log.Debug("value: %s", *mapValue)

			if k == "Temperature" {
				a.AC.Temperature = (float64(*mapValue) - 500) / 10
			} else if k == "AcTargetSetpoint" {
				a.AC.AcTargetSetpoint = int(*mapValue)
			} else if k == "AcMode" {
				if int(*mapValue) == 3 {
					a.AC.AcMode = "Fan"
				} else if int(*mapValue) == 4 {
					a.AC.AcMode = "Cool"
				} else if int(*mapValue) == 1 {
					a.AC.AcMode = "Heat"
				} else { // TODO: Map the rest of the modes.
					a.AC.AcMode = strconv.Itoa(int(*mapValue))
				}
			} else if k == "Spill" {
				if int(*mapValue) == 0 {
					a.AC.Spill = false
				} else {
					a.AC.Spill = true
				}
			}
		}
	}

	return nil
}

// FixOpenPercentages fixes the spill group's open percentage.
func (a *AirTouch) FixOpenPercentages() {
	totalOpen := 0
	totalSpillGroups := 0

	for i := range a.Groups {
		// Closed groups seem to report their last OpenPercentage value, rather than 0 which is what a closed group should be.
		// A spill group cannot be closed but can be off.
		if !a.Groups[i].Spill && a.Groups[i].PowerState == "Off" {
			a.Groups[i].OpenPercentage = 0
		}

		// The total OpenPercentage excluding spill groups.
		if !a.Groups[i].Spill {
			totalOpen += a.Groups[i].OpenPercentage
		}

		// Total number of spill groups to divide 100 - OpenPercentage between.
		if a.Groups[i].Spill {
			totalSpillGroups++
		}
	}

	log.Printf("Total open percentage = %d", totalOpen)
	log.Printf("Total spill groups = %d", totalSpillGroups)

	// Now fix up the OpenPercentage for the spill groups.
	for i := range a.Groups {
		if a.Groups[i].Spill {
			a.Groups[i].SpillPercentage = (a.Groups[i].OpenPercentage - (100 - (totalOpen / totalSpillGroups))) * -1
			//Adjusted spill group %s OpenPercentage from %d to %d", a.Groups[i].Name, oldSpillPct, a.Groups[i].OpenPercentage)
		}
	}
}

// DecodeGroupStatusMessage decodes each zones status. Each zone has many attibutes which are
// extracted and typed accordingly.
func (a *AirTouch) DecodeGroupStatusMessage(response MessageOutput) error {
	//a.Log.Debug("groupstatus: %v", response.Body)
	packetInfoLocationMap := a.GroupStatusMap()

	var tempGroups []Group

	for i, chunk := range chunk(response.Body, 6) {
		// TODO: Unhardcode 4 as it implies we should only consider 4 chunks (zones).
		// Or make it configurable?
		if i > 3 {
			break
		}

		// groupNumber, err := a.TranslateMapValueToValue(chunk, packetInfoLocationMap["GroupNumber"])
		// if err != nil {
		// 	return nil, err
		// }
		//a.Log.Debug("--------------------------------")
		//a.Log.Debug("groupNumber: %d", *groupNumber)

		var group Group

		for k := range packetInfoLocationMap {

			mapValue, err := a.TranslateMapValueToValue(chunk, packetInfoLocationMap[k])
			if err != nil {
				return err
			}

			//a.Log.Debug("key: %s", k)
			//a.Log.Debug("value: %s", *mapValue)

			//var value float64

			if k == "Temperature" {
				//a.Log.Debug("value: %d", *mapValue)
				group.Temperature = (float64(*mapValue) - 500) / 10
				//a.Log.Debug("%s: %.1f", k, group.Temperature)
			} else if k == "GroupNumber" {
				group.Number = int(*mapValue)
			} else if k == "PowerState" {
				if int(*mapValue) == 0 {
					group.PowerState = "Off"
				} else if int(*mapValue) == 1 {
					group.PowerState = "On"
				} else {
					group.PowerState = "Turbo"
				}
			} else if k == "ControlMethod" {
				if int(*mapValue) == 0 {
					group.ControlMethod = "PercentageControl"
				} else {
					group.ControlMethod = "TemperatureControl"
				}
			} else if k == "OpenPercentage" {
				group.OpenPercentage = int(*mapValue)
			} else if k == "TargetSetpoint" {
				group.TargetSetpoint = int(*mapValue)
			} else if k == "Spill" {
				if int(*mapValue) == 0 {
					group.Spill = false
				} else {
					group.Spill = true
				}
			}
		}
		tempGroups = append(tempGroups, group)
	}

	a.Groups = tempGroups

	return nil
}

// AddMapValueToBinaryValue adds map value to binary value.
func (a *AirTouch) AddMapValueToBinaryValue(binaryMessagePayloadString string, attribute string, value string) (*string, error) {
	//a.Log.Debug("Location map = %s", attribute)
	//a.Log.Debug("Value to set = %s", value)

	length := 8

	firstValue := strings.Split(attribute, ":")[0]
	//a.Log.Debug("firstValue: %s", firstValue)

	secondValue := strings.Split(attribute, ":")[1]
	//a.Log.Debug("secondValue: %s", secondValue)

	lowerValue, err := strconv.Atoi(strings.Split(secondValue, "-")[0])
	if err != nil {
		return nil, err
	}
	//a.Log.Debug("lowerValue: %d", lowerValue)

	upperValue, err := strconv.Atoi(strings.Split(secondValue, "-")[1])
	if err != nil {
		return nil, err
	}
	//a.Log.Debug("upperValue: %d", upperValue)

	// Spec counts bytes backwards so so do we.
	bitmaskStart := length - (upperValue - 1)
	//a.Log.Debug("bitmaskStart: %d", bitmaskStart)

	bitmaskEnd := length - (lowerValue - 1)
	//a.Log.Debug("bitmaskEnd: %d", bitmaskEnd)

	// binaryMessage needs to be at least as long as (byteNumber - 1) * 8 + bitmaskstart, so add as many zeroes as required to make that happen

	firstValueAsInt, err := strconv.Atoi(firstValue)
	if err != nil {
		return nil, err
	}
	//a.Log.Debug("firstValueAsInt: %d", firstValueAsInt)

	// while(len(binaryMessagePayloadString) < (byteNumber - 1) * 8 + (bitmaskstart - 1)):
	// binaryMessagePayloadString += "0"

	//a.Log.Debug("len(binaryMessagePayloadString): %d", len(binaryMessagePayloadString))

	for len(binaryMessagePayloadString) < (firstValueAsInt-1)*8+(bitmaskStart-1) {
		binaryMessagePayloadString += "0"
	}
	//a.Log.Debug("binaryMessagePayloadStringLoop: %s", binaryMessagePayloadString)

	//binOfValueAsString = bin(value)[2:];
	valueAsInt, err := strconv.Atoi(value)
	if err != nil {
		return nil, err
	}

	binOfValueAsString := strconv.FormatInt(int64(valueAsInt), 2)[0:]
	//a.Log.Debug("binOfValueAsString: %s", binOfValueAsString)

	lengthNeededForBinValue := bitmaskEnd - (bitmaskStart - 1)
	//a.Log.Debug("lengthNeededForBinValue: %d", lengthNeededForBinValue)

	binaryMessagePayloadString = binaryMessagePayloadString + "00000000"[0:lengthNeededForBinValue-len(binOfValueAsString)] + binOfValueAsString
	//a.Log.Debug("binaryMessagePayloadString: %s", binaryMessagePayloadString)

	return &binaryMessagePayloadString, nil
}

// TranslateMapValueToValue takes a chunk and the position where we expect values to be located
// and translates this to an actual value.
func (a *AirTouch) TranslateMapValueToValue(chunk []byte, packetInfoLocation string) (*int64, error) {
	//a.Log.Debug("locationMap: %s", packetInfoLocation)
	//a.Log.Debug("chunk: %v", chunk)
	length := 8

	firstValue := strings.Split(packetInfoLocation, ":")[0]
	//a.Log.Debug("firstValue: %s", firstValue)

	secondValue := strings.Split(packetInfoLocation, ":")[1]
	//a.Log.Debug("secondValue: %s", secondValue)

	byteNumber, err := strconv.Atoi(firstValue)
	if err != nil {
		return nil, err
	}
	//a.Log.Debug("byteNumber: %d", byteNumber)

	lowerValue, err := strconv.Atoi(strings.Split(secondValue, "-")[0])
	if err != nil {
		return nil, err
	}
	//a.Log.Debug("lowerValue: %d", lowerValue)

	upperValue, err := strconv.Atoi(strings.Split(secondValue, "-")[1])
	if err != nil {
		return nil, err
	}
	//a.Log.Debug("upperValue: %d", upperValue)

	if upperValue > 8 {
		length = 16
		//a.Log.Debug("upperValue > 8 so length changed to 16")
	}

	// Spec counts bytes backwards so so do we.
	bitmaskStart := length - (upperValue - 1)
	//a.Log.Debug("bitmaskStart: %d", bitmaskStart)
	bitmaskEnd := length - (lowerValue - 1)
	//a.Log.Debug("bitmaskEnd: %d", bitmaskEnd)

	//a.Log.Debug("raw: %v", chunk[byteNumber-1])
	//a.Log.Debug("hex: %s", fmt.Sprintf("%x", chunk[byteNumber-1]))
	byteToInt, err := strconv.Atoi(fmt.Sprintf("%v", chunk[byteNumber-1]))
	if err != nil {
		return nil, err
	}
	//a.Log.Debug("byteToInt: %d", byteToInt)

	// Convert to binary.
	byteAsString := strconv.FormatInt(int64(byteToInt), 2)
	//a.Log.Debug("byteAsString: %s", byteAsString)
	byteStringAdjusted := "00000000"[0:8-len(byteAsString)] + byteAsString[0:]
	//a.Log.Debug("byteStringAdjusted: %s", byteStringAdjusted)

	if length > 8 {

		byteNumberToBinary := strconv.FormatInt(int64(chunk[byteNumber]), 2)
		//a.Log.Debug("byteNumberToBinary: %s", byteNumberToBinary)
		//a.Log.Debug("firstPart: %d", 8-(len(byteNumberToBinary)))
		//a.Log.Debug("secondPart: %d", byteNumberToBinary)

		byteStringAdjusted += ("00000000"[0:8-(len(byteNumberToBinary))] + byteNumberToBinary)
		//a.Log.Debug("length > 8 so byteStringAdjusted: %s", byteStringAdjusted)
	}

	byteSegment := byteStringAdjusted[bitmaskStart-1 : bitmaskEnd]
	//a.Log.Debug("byteSegment: %s", byteSegment)

	byteSegmentAsValue, err := strconv.ParseInt(byteSegment, 2, 64)
	if err != nil {
		return nil, err
	}
	//a.Log.Debug("byteSegmentAsValue: %d", byteSegmentAsValue)

	return &byteSegmentAsValue, nil
}

func chunk(buf []byte, lim int) [][]byte {
	var chunk []byte
	chunks := make([][]byte, 0, len(buf)/lim+1)

	for len(buf) >= lim {
		chunk, buf = buf[:lim], buf[lim:]
		chunks = append(chunks, chunk)
	}

	if len(buf) > 0 {
		chunks = append(chunks, buf[:])
	}

	return chunks
}
