package flamenco

import (
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/mgo.v2/bson"
)

const timeOfDayStringFormat = "%02d:%02d"

// TimeOfDay is marshalled as 'HH:MM'.
// Its date and timezone components are ignored, and the time is supposed
// to be interpreted as local time on any date (f.e. a scheduled sleep time
// of some Worker on a certain day-of-week & local timezone).
type TimeOfDay struct {
	Hour   int
	Minute int
}

// MakeTimeOfDay converts a time.Time into a TimeOfDay.
func MakeTimeOfDay(someTime time.Time) TimeOfDay {
	return TimeOfDay{someTime.Hour(), someTime.Minute()}
}

// Equals returns True iff both times represent the same time of day.
func (ot TimeOfDay) Equals(other TimeOfDay) bool {
	return ot.Hour == other.Hour && ot.Minute == other.Minute
}

// IsBefore returns True iff ot is before other.
// Ignores everything except hour and minute fields.
func (ot TimeOfDay) IsBefore(other TimeOfDay) bool {
	if ot.Hour != other.Hour {
		return ot.Hour < other.Hour
	}
	return ot.Minute < other.Minute
}

// IsAfter returns True iff ot is after other.
// Ignores everything except hour and minute fields.
func (ot TimeOfDay) IsAfter(other TimeOfDay) bool {
	if ot.Hour != other.Hour {
		return ot.Hour > other.Hour
	}
	return ot.Minute > other.Minute
}

// OnDate returns the time of day in the local timezone on the given date.
func (ot TimeOfDay) OnDate(date time.Time) time.Time {
	year, month, day := date.Date()
	return time.Date(year, month, day, ot.Hour, ot.Minute, 0, 0, time.Local)
}

func (ot TimeOfDay) String() string {
	return fmt.Sprintf(timeOfDayStringFormat, ot.Hour, ot.Minute)
}

// UnmarshalJSON turns a "HH:MM" string into a time.Time instance.
func (ot *TimeOfDay) UnmarshalJSON(b []byte) error {
	var asString string
	if err := json.Unmarshal(b, &asString); err != nil {
		return err
	}
	return ot.setString(asString)
}

// MarshalJSON turns a time.Time instance into a "HH:MM" string.
func (ot TimeOfDay) MarshalJSON() ([]byte, error) {
	asBytes, err := json.Marshal(ot.String())
	return asBytes, err
}

// We can't return the string itself (which is what I would want), because
// TimeOfDay is a struct, and the BSON module thus serialises it to a
// subdocument and not a value.
type onlyTimeBSON struct {
	Time string
}

// SetBSON turns BSON an TimeOfDay object.
func (ot *TimeOfDay) SetBSON(raw bson.Raw) error {
	var decoded onlyTimeBSON
	err := raw.Unmarshal(&decoded)
	if err != nil {
		return err
	}
	return ot.setString(decoded.Time)
}

// GetBSON turns a time.Time instance into BSON.
func (ot TimeOfDay) GetBSON() (interface{}, error) {
	return onlyTimeBSON{ot.String()}, nil
}

func (ot *TimeOfDay) setString(value string) error {
	scanned := TimeOfDay{}
	_, err := fmt.Sscanf(value, timeOfDayStringFormat, &scanned.Hour, &scanned.Minute)
	if err != nil {
		return err
	}
	*ot = scanned
	return nil
}
