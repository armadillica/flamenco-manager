package flamenco

/* ***** BEGIN MIT LICENSE BLOCK *****
 * (c) 2019, Blender Foundation - Sybren A. Stüvel
 *
 * Permission is hereby granted, free of charge, to any person obtaining
 * a copy of this software and associated documentation files (the
 * "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish,
 * distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so, subject to
 * the following conditions:
 *
 * The above copyright notice and this permission notice shall be
 * included in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 * IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
 * CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
 * TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 * ***** END MIT LICENCE BLOCK *****
 */

import (
	"encoding/json"
	"time"

	"github.com/stretchr/testify/assert"

	check "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type TimeOfDayTestSuite struct {
}

var _ = check.Suite(&TimeOfDayTestSuite{})

func (s *TimeOfDayTestSuite) TestIsBefore(c *check.C) {
	test := func(expect bool, hour1, min1, hour2, min2 int) {
		time1 := TimeOfDay{hour1, min1}
		time2 := TimeOfDay{hour2, min2}

		assert.Equal(c, expect, time1.IsBefore(time2))
	}
	test(false, 0, 0, 0, 0)
	test(true, 0, 0, 0, 1)
	test(true, 1, 59, 2, 0)
	test(true, 1, 2, 1, 3)
	test(true, 1, 2, 15, 1)
	test(false, 17, 0, 8, 0)
}

func (s *TimeOfDayTestSuite) TestIsAfter(c *check.C) {
	test := func(expect bool, hour1, min1, hour2, min2 int) {
		time1 := TimeOfDay{hour1, min1}
		time2 := TimeOfDay{hour2, min2}

		assert.Equal(c, expect, time1.IsAfter(time2))
	}
	test(false, 0, 0, 0, 0)
	test(true, 0, 1, 0, 0)
	test(true, 2, 1, 1, 59)
	test(true, 1, 3, 1, 2)
	test(true, 15, 1, 1, 2)
	test(false, 8, 0, 17, 0)
}

func (s *TimeOfDayTestSuite) TestOnDate(c *check.C) {
	theDate := time.Date(2018, 12, 13, 7, 59, 43, 123, time.Local)
	tod := TimeOfDay{16, 47}
	expect := time.Date(2018, 12, 13, 16, 47, 0, 0, time.Local)
	assert.Equal(c, expect, tod.OnDate(theDate))

	// Midnight on the same day.
	tod = TimeOfDay{0, 0}
	expect = time.Date(2018, 12, 13, 0, 0, 0, 0, time.Local)
	assert.Equal(c, expect, tod.OnDate(theDate))

	// Midnight a day later.
	tod = TimeOfDay{24, 0}
	expect = time.Date(2018, 12, 14, 0, 0, 0, 0, time.Local)
	assert.Equal(c, expect, tod.OnDate(theDate))

}

func (s *TimeOfDayTestSuite) TestMarshalJSON(c *check.C) {
	test := func(expected string, toTest TimeOfDay) {
		marshalled, err := json.Marshal(&toTest)
		assert.Nil(c, err)
		assert.Equal(c, expected, string(marshalled))
	}

	test("\"02:34\"", TimeOfDay{2, 34})
	test("\"18:34\"", TimeOfDay{18, 34})
	test("\"18:03\"", TimeOfDay{18, 3})
}

func (s *TimeOfDayTestSuite) TestUnmarshalJSON(c *check.C) {
	test := func(hour, minute int, toTest string) {
		var timeOfDay TimeOfDay
		err := json.Unmarshal([]byte(toTest), &timeOfDay)
		assert.Nil(c, err)

		assert.Equal(c, hour, timeOfDay.Hour)
		assert.Equal(c, minute, timeOfDay.Minute)
	}

	testError := func(toTest string) {
		var timeOfDay TimeOfDay
		err := json.Unmarshal([]byte(toTest), &timeOfDay)
		assert.NotNil(c, err)
	}

	test(2, 34, "\"02:34\"")
	test(16, 34, "\"16:34\"")
	test(16, 4, "\"16:04\"")
	test(16, 4, "\"16:4\"")
	test(16, 34, "\"16:34:44\"")

	testError("\"16:\"")
	testError("\"je moeder\"")
}

func (s *TimeOfDayTestSuite) TestBSON(c *check.C) {
	test := func(hour, minute int) {
		asToD := TimeOfDay{hour, minute}
		marshalled, err := bson.Marshal(&asToD)
		assert.Nil(c, err)

		var fromBSON TimeOfDay
		err = bson.Unmarshal(marshalled, &fromBSON)
		if err != nil {
			assert.FailNow(c, "err is not nil", err.Error())
			return
		}

		assert.Equal(c, hour, fromBSON.Hour)
		assert.Equal(c, minute, fromBSON.Minute)
	}

	test(2, 34)
	test(16, 34)
	test(16, 3)
	test(0, 0)
}
