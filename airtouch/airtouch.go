package airtouch

// AirTouch models AC and groups.
type AirTouch struct {
	IPAddress        string
	Port             int
	RootTempDir      string
	Timezone         string
	ReportLoopPeriod int
	AC               AC
	Groups           []Group
}

// CommunicateMessage takes a message and translates the return reply.
func (a *AirTouch) CommunicateMessage(message *MessageInput) (*MessageOutput, error) {
	err := a.PrepareMessage(message)
	if err != nil {
		return nil, err
	}

	//a.Log.Debug("Message: %s", message.MessageWithCRC)

	responseBytes, err := a.SendMessage(&message.MessageWithCRC)
	if err != nil {
		return nil, err
	}

	//a.Log.Debug("Response: %s", responseBytes)

	messageOut, err := a.TranslatePacketToMessage(responseBytes)
	if err != nil {
		return nil, err
	}

	return &messageOut, nil
}
