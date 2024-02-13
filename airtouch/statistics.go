package airtouch

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GenerateGroupStatistics calculates how long each group has been turned on for the day.
func (a *AirTouch) GenerateGroupStatistics() error {
	localTime, err := a.LocalTime()
	if err != nil {
		return err
	}

	timeToResetFiles, _ := a.EndOfDay()
	if timeToResetFiles {
		log.Printf("Removing airtouch group activity files")
		files, err := filepath.Glob(fmt.Sprintf("%s/%s", a.RootTempDir, "airtouch_*"))
		if err != nil {
			return err
		}

		for _, f := range files {
			log.Printf("Removing %s...", f)
			if err := os.Remove(f); err != nil {
				return err
			}
		}
	}

	for i, g := range a.Groups {
		filename := fmt.Sprintf("airtouch_%s_activity", g.Name)

		// Room requires heating/cooling.
		// Needs to be above 50 as that is the default on percentage from Fan -> Heat/Cool.
		// When that transition happens, we don't want to record that there is active heating/cooling occurring.
		if a.AC.AcMode != "Fan" && g.PowerState == "On" && g.OpenPercentage > 50 {
			err = a.AppendValueToFile(filename, fmt.Sprintf("%s,%s\n", g.PowerState, localTime.Format(time.RFC3339)))
			if err != nil {
				log.Printf("Unable to write group activity to file, please correct, skipping statistics")
				return err
			}
		} else {
			// Off is hardcoded here because a room might be On, with openPercentage = 10. In this case, we can't use the
			// variable g.PowerState as we're making a decision here that the A/C is not actually On for this room, it's just
			// the minimum vent percentage is stopping the vent from fully closing.
			err = a.AppendValueToFile(filename, fmt.Sprintf("%s,%s\n", "Off", localTime.Format(time.RFC3339)))
			if err != nil {
				log.Printf("Unable to write group activity to file, please correct, skipping statistics")
				return err
			}
		}
		// For each group, go through their activity file and add up the time between the first
		// On and next Off.

		f, err := os.Open(fmt.Sprintf("%s/%s", a.RootTempDir, filename))
		if err != nil {
			return err
		}
		defer f.Close()

		// Splits on newlines by default.
		scanner := bufio.NewScanner(f)

		startTime := *localTime
		endTime := *localTime
		durationTotalMins := 0.0
		duration := 0.0
		// https://golang.org/pkg/bufio/#Scanner.Scan
		foundOnBlock := false
		foundOffBlock := false
		//a.Log.Debug("working with %s", g.Name)
		//a.Log.Debug("-------------------------------------------------")
		for scanner.Scan() {

			line := strings.Split(scanner.Text(), ",")
			state := line[0]
			timeStamp := line[1]

			if !foundOnBlock && state == "On" {
				//a.Log.Debug("found On block %s", timeStamp)
				startTime, err = time.Parse("2006-01-02T15:04:05Z07:00", timeStamp)
				if err != nil {
					return err
				}
				foundOnBlock = true
			} else if foundOnBlock && !foundOffBlock && state == "Off" {
				//a.Log.Debug("found Off block %s", timeStamp)
				endTime, err = time.Parse("2006-01-02T15:04:05Z07:00", timeStamp)
				if err != nil {
					return err
				}
				foundOffBlock = true
			}

			if foundOnBlock && foundOffBlock {
				duration = endTime.Sub(startTime).Minutes()
				//a.Log.Debug("duration of this block is %f minutes", duration)
				durationTotalMins += duration
				foundOnBlock = false
				foundOffBlock = false
			}
		}

		// Group hasn't been Off yet
		if foundOnBlock && !foundOffBlock {
			durationTotalMins += time.Since(startTime).Minutes()
			//a.Log.Debug("didn't found an Off block so assuming this has always been on")
		}

		a.Groups[i].DayDurationMinutes = durationTotalMins
		log.Printf("%s has been On for %f minutes today", a.Groups[i].Name, a.Groups[i].DayDurationMinutes)
	}

	return nil
}
