package radioshark

import (
	"errors"
	"github.com/jsouthworth/hid"
	"strconv"
)

const (
	VENDOR_ID         = 0x077D
	PRODUCT_ID        = 0x627A
	ENDPOINT          = 0x5
	pktLength         = 6
	freqCode          = 0xC0
	blueIntensityCode = 0xA0
	bluePulseCode     = 0xA1
	redOnCode         = 0xA9
	redOffCode        = 0xA8
)

func makeHIDPacket() []byte {
	return make([]byte, pktLength)
}

func makeFreqPacket(freq uint16) []byte {
	pkt := makeHIDPacket()
	pkt[0] = freqCode
	pkt[2] = uint8(freq>>8) & 0xFF
	pkt[3] = uint8(freq) & 0xFF
	return pkt
}

func ParseFMFrequency(freq string) (uint16, error) {
	parsedFreq, err := strconv.ParseFloat(freq, 32)
	if err != nil {
		return 0, err
	}
	if parsedFreq < 88.0 || parsedFreq > 108.0 {
		return 0, errors.New("FM frequency must be between 88.0 and 108.0")
	}
	return uint16(((parsedFreq*1000)+10701)/12.5) + 3, nil
}

func ParseAMFrequency(freq string) (uint16, error) {
	parsedFreq, err := strconv.Atoi(freq)
	if err != nil {
		return 0, err
	}
	if parsedFreq < 535 || parsedFreq > 1705 {
		return 0, errors.New("AM frequency must be between 535 and 1705")
	}
	return uint16(float32(parsedFreq)) + 450, nil
}

func ValidateBlueLEDIntensity(intensity uint8) error {
	if intensity > 127 {
		return errors.New("intensity must be less than 127")
	}
	return nil
}

func ValidateBlueLEDPulse(rate uint8) error {
	if rate > 127 {
		return errors.New("rate must be less than 127")
	}
	return nil
}

func ValidateModulation(modulation string) error {
	switch modulation {
	case "am", "AM", "fm", "FM":
		return nil
	}
	return errors.New("unknown modulation " + modulation)
}

type RadioShark struct {
	dev hid.Device
}

func List() []string {
	out := make([]string, 0)
	for devInfo := range hid.FindDevices(VENDOR_ID, PRODUCT_ID) {
		out = append(out, devInfo.Path)
	}
	return out
}

func Open(path string) (*RadioShark, error) {
	devInfo, err := hid.ByPath(path)
	if err != nil {
		return nil, err
	}
	dev, err := devInfo.Open()
	if err != nil {
		return nil, err
	}
	return &RadioShark{dev: dev}, nil
}

func (shark *RadioShark) Close() {
	shark.dev.Close()
}

func (shark *RadioShark) writeHIDPacket(pkt []byte) error {
	_, err := shark.dev.WriteInterrupt(ENDPOINT, pkt)
	return err
}

func (shark *RadioShark) SetFrequency(modulation, freq string) error {
	err := ValidateModulation(modulation)
	if err != nil {
		return err
	}
	switch modulation {
	case "FM", "fm":
		freqVal, err := ParseFMFrequency(freq)
		if err != nil {
			return err
		}
		return shark.SetFMFrequency(freqVal)
	case "AM", "am":
		freqVal, err := ParseAMFrequency(freq)
		if err != nil {
			return err
		}
		return shark.SetAMFrequency(freqVal)
	}
	return nil
}

func (shark *RadioShark) SetFMFrequency(freq uint16) error {
	return shark.writeHIDPacket(makeFreqPacket(freq))
}

func (shark *RadioShark) SetAMFrequency(freq uint16) error {
	pkt := makeFreqPacket(freq)
	pkt[1] = 0x12
	return shark.writeHIDPacket(pkt)
}

func (shark *RadioShark) SetBlueLEDIntensity(intensity uint8) error {
	err := ValidateBlueLEDIntensity(intensity)
	if err != nil {
		return err
	}
	pkt := make([]byte, pktLength)
	pkt[0] = blueIntensityCode
	pkt[1] = byte(intensity)
	return shark.writeHIDPacket(pkt)
}

func (shark *RadioShark) SetBlueLEDPulse(rate uint8) error {
	err := ValidateBlueLEDPulse(rate)
	if err != nil {
		return err
	}
	pkt := make([]byte, pktLength)
	pkt[0] = bluePulseCode
	pkt[1] = byte(rate)
	return shark.writeHIDPacket(pkt)
}

func (shark *RadioShark) SetRedLED(toggle bool) error {
	pkt := make([]byte, pktLength)
	if toggle {
		pkt[0] = redOnCode
		pkt[1] = 1
	} else {
		pkt[0] = redOffCode
		pkt[1] = 0
	}
	return shark.writeHIDPacket(pkt)
}
