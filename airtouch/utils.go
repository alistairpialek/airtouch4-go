package airtouch

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"time"
)

// ReadStringFromFile reads a value from filename.
func (a *AirTouch) ReadStringFromFile(filename string) (string, error) {
	if a.FileExists(filename) {
		valueString, fileErr := ioutil.ReadFile(fmt.Sprintf("%s/%s", a.RootTempDir, filename))
		if fileErr != nil {
			return "", fileErr
		}
		return string(valueString), nil
	}
	return "", errors.New("file does not exist")
}

// WriteValueToFile writes a value to filename.
func (a *AirTouch) WriteValueToFile(filename string, value string) error {
	content := []byte(value)
	err := ioutil.WriteFile(fmt.Sprintf("%s/%s", a.RootTempDir, filename), content, 0644)

	if err != nil {
		return err
	}

	return nil
}

// FileExists returns true if a filepath exists.
func (a *AirTouch) FileExists(name string) bool {
	if _, err := os.Stat(fmt.Sprintf("%s/%s", a.RootTempDir, name)); err == nil {
		return true
	} else if os.IsNotExist(err) {
		return false
	} else {
		return false
	}
}

// LocalTime sets a time in location per specified config.
func (a *AirTouch) LocalTime() (*time.Time, error) {
	loc, err := time.LoadLocation(a.Timezone)
	if err != nil {
		return nil, err
	}

	localTime := time.Now().In(loc)
	return &localTime, nil
}

// EndOfDay returns true if hour is 23 and first reportLoopPeriod.
func (a *AirTouch) EndOfDay() (bool, error) {
	time, err := a.LocalTime()
	if err != nil {
		return false, err
	}

	return time.Hour() == 23 &&
		time.Minute() == 59 &&
		(time.Second() >= 0 && time.Second() <= a.ReportLoopPeriod), nil
}

// AppendValueToFile appends a value to filename.
func (a *AirTouch) AppendValueToFile(filename, value string) error {
	f, err := os.OpenFile(fmt.Sprintf("%s/%s", a.RootTempDir, filename), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)

	if err != nil {
		return err
	}

	defer f.Close()

	if _, err = f.WriteString(value); err != nil {
		return err
	}

	return nil
}
