package airtouch

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

// acStatusBody is an ACStatus reply body: AC 0 on, Fan at Low, setpoint 21,
// 23.0C.
var acStatusBody = []byte{0x40, 0x32, 0x15, 0x00, 0x5b, 0x40, 0x00, 0x00}

// frame builds a reply frame with a correct length and checksum.
func frame(id, msgType byte, body []byte) []byte {
	f := []byte{0x55, 0x55, 0xb0, 0x80, id, msgType, 0, 0}
	binary.BigEndian.PutUint16(f[6:8], uint16(len(body)))
	f = append(f, body...)
	return binary.BigEndian.AppendUint16(f, checksum(f[2:]))
}

// serve answers the first connection with each of writes, pausing between
// them so the client sees separate segments.
func serve(t *testing.T, writes ...[]byte) *AirTouch {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })

	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.Read(make([]byte, 64))
		for _, w := range writes {
			conn.Write(w)
			time.Sleep(50 * time.Millisecond)
		}
	}()

	return &AirTouch{IPAddress: "127.0.0.1", Port: l.Addr().(*net.TCPAddr).Port}
}

func TestChecksumMatchesKnownRequest(t *testing.T) {
	// From the console's protocol: GroupStatus is sent as 5555 80b0012b0000 f52f.
	if got := checksum([]byte{0x80, 0xb0, 0x01, 0x2b, 0x00, 0x00}); got != 0xf52f {
		t.Errorf("checksum = %04x, want f52f", got)
	}
}

func TestSplitReplyIsReadWhole(t *testing.T) {
	f := frame(0x01, 0x2d, acStatusBody)
	a := serve(t, f[:5], f[5:11], f[11:])

	if err := a.GetACStatus(); err != nil {
		t.Fatal(err)
	}

	if a.AC.Temperature != 23 || a.AC.AcTargetSetpoint != 21 || a.AC.AcMode != "Fan" {
		t.Errorf("decoded %+v", a.AC)
	}
}

func TestReplyEndingEarlyIsAnError(t *testing.T) {
	f := frame(0x01, 0x2d, acStatusBody)
	a := serve(t, f[:12])

	err := a.GetACStatus()
	if err == nil || !strings.Contains(err.Error(), "body of 8 bytes") {
		t.Fatalf("err = %v, want a short body", err)
	}
}

func TestTranslateRefusesBadReplies(t *testing.T) {
	good := frame(0x01, 0x2d, acStatusBody)

	corrupt := append([]byte{}, good...)
	corrupt[12] ^= 0x01

	long := append(append([]byte{}, good...), 0x00)

	cases := map[string]struct {
		reply []byte
		want  string
	}{
		"shorter than a header": {good[:6], "shorter than an empty frame"},
		"wrong frame header":    {append([]byte{0x00, 0x00}, good[2:]...), "reply header"},
		"longer than declared":  {long, "declares a 8 byte body but is 19 bytes"},
		"shorter than declared": {good[:len(good)-3], "declares a 8 byte body but is 15 bytes"},
		"corrupted body":        {corrupt, "checksum"},
	}

	a := &AirTouch{}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := a.TranslatePacketToMessage(c.reply)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want %q", err, c.want)
			}
		})
	}

	out, err := a.TranslatePacketToMessage(good)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Body) != len(acStatusBody) {
		t.Errorf("body is %d bytes, want %d", len(out.Body), len(acStatusBody))
	}
}

func TestFrameOfAnotherTypeIsSkipped(t *testing.T) {
	a := serve(t, frame(0x01, 0x2f, make([]byte, 6)), frame(0x01, 0x2d, acStatusBody))

	if err := a.GetACStatus(); err != nil {
		t.Fatal(err)
	}

	if a.AC.Temperature != 23 {
		t.Errorf("decoded %+v, want the 2d reply rather than the 2f frame", a.AC)
	}
}

func TestOnlyFramesOfAnotherTypeIsAnError(t *testing.T) {
	a := serve(t, frame(0x01, 0x2f, make([]byte, 6)))

	err := a.GetACStatus()
	if err == nil || !strings.Contains(err.Error(), "reply type is 2f, expected 2d, then reading the next reply") {
		t.Fatalf("err = %v, want a type mismatch", err)
	}
}

func TestCorruptFrameIsNotSkipped(t *testing.T) {
	corrupt := frame(0x01, 0x2f, make([]byte, 6))
	corrupt[9] ^= 0x01
	a := serve(t, corrupt, frame(0x01, 0x2d, acStatusBody))

	err := a.GetACStatus()
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v, want a checksum error", err)
	}
}

func TestControlIsAnsweredWithStatus(t *testing.T) {
	for request, want := range map[byte]byte{0x2a: 0x2b, 0x2c: 0x2d, 0x2b: 0x2b, 0x1f: 0x1f} {
		if got := replyType([]byte{0x80, 0xb0, 0x01, request}); got != want {
			t.Errorf("replyType(%02x) = %02x, want %02x", request, got, want)
		}
	}
}

func TestGroupNameForUnreportedGroupIsAnError(t *testing.T) {
	a := &AirTouch{Groups: make([]Group, 2)}
	body := append([]byte{0xff, 0x12, 0x05}, []byte("Living\x00\x00")...)

	err := a.DecodeGroupNameMessage(MessageOutput{Body: body})
	if err == nil || !strings.Contains(err.Error(), "names group 5") {
		t.Fatalf("err = %v, want an unreported group", err)
	}
}

func TestShortChunkIsAnError(t *testing.T) {
	a := &AirTouch{}
	if _, err := a.TranslateMapValueToValue([]byte{0x40, 0x32, 0x15, 0x00, 0x57}, "5:6-16"); err == nil {
		t.Error("want an error for a 16 bit value past the end of the chunk")
	}
}
