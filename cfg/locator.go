package cfg

import (
	"regexp"
	"strings"

	"github.com/pd0mz/go-maidenhead"
)

// MaidenheadLocator is a validated Maidenhead grid locator.
type MaidenheadLocator string

// MaidenheadPoint is a parsed MaidenheadLocator.
type MaidenheadPoint = maidenhead.Point

const locatorPattern = `^[A-Ra-r]{2}[0-9]{2}[A-Xa-x]{2}$`

// Valid returns true if the locator is a valid six-character Maidenhead locator.
func (l MaidenheadLocator) Valid() bool {
	if l == "" {
		return false
	}
	matched, _ := regexp.MatchString(locatorPattern, string(l))
	return matched
}

// String returns the locator in uppercase.
func (l MaidenheadLocator) String() string {
	return strings.ToUpper(string(l))
}

// Point returns the coordinates as parsed from the locator string.
func (l MaidenheadLocator) Point() (MaidenheadPoint, error) {
	pt, err := maidenhead.ParseLocator(string(l))
	if err != nil {
		return MaidenheadPoint{}, err
	}
	return MaidenheadPoint(pt), nil
}
