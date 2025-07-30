package voacap

import (
	"fmt"
	"io"
	"math"
	"time"

	"github.com/pd0mz/go-maidenhead"
)

// EncodeParams holds the input parameters for a VOACAP prediction.
type EncodeParams struct {
	DateTime      time.Time
	From          string
	To            string
	SSN           int
	TransmitPower int
	MinSNR        int
	Frequency     float64
}

// Encode writes the input data for a VOACAP prediction to the given writer.
func Encode(w io.Writer, params EncodeParams) error {
	from, err := maidenhead.ParseLocator(params.From)
	if err != nil {
		return fmt.Errorf("error converting 'from' Maidenhead locator: %w", err)
	}
	to, err := maidenhead.ParseLocator(params.To)
	if err != nil {
		return fmt.Errorf("error converting 'to' Maidenhead locator: %w", err)
	}

	// VOACAP expects time in UTC
	params.DateTime = params.DateTime.UTC()

	writeCommentCard(w, "Any VOACAP default cards may be placed in the file: VOACAP.DEF")
	writeLineMaxCard(w, 55)
	writeCoeffsCard(w, "CCIR")
	writeTimeCard(w, params.DateTime.Hour(), params.DateTime.Hour(), 1, 1)
	writeMonthCard(w, params.DateTime.Year(), params.DateTime.Month())
	writeSunspotCard(w, params.SSN)
	writeLabelCard(w, params.From, params.To)
	writeCircuitCard(w, from.Latitude, from.Longitude, to.Latitude, to.Longitude, 0)
	writeSystemCard(w, float64(params.MinSNR))
	writeFprobCard(w)
	writeAntennaCard(w, 1, 1, 2, 30, params.Frequency, "", 90.0, float64(params.TransmitPower))
	writeAntennaCard(w, 2, 2, 2, 30, params.Frequency, "", 270.0, float64(params.TransmitPower))
	writeFrequencyCard(w, params.Frequency)
	writeMethodCard(w, 30, 0)
	writeExecuteCard(w)
	writeQuitCard(w)
	return nil
}

func getLatHemi(lat float64) string {
	if lat < 0 {
		return "S"
	}
	return "N"
}

func getLonHemi(lon float64) string {
	if lon < 0 {
		return "W"
	}
	return "E"
}

func writeCommentCard(w io.Writer, comment string) {
	fmt.Fprintf(w, "COMMENT    %-74s\n", comment)
}

func writeLineMaxCard(w io.Writer, maxLines int) {
	fmt.Fprintf(w, "LINEMAX   %5d\n", maxLines)
}

func writeCoeffsCard(w io.Writer, coeffs string) {
	fmt.Fprintf(w, "COEFFS    %-4s\n", coeffs)
}

func writeTimeCard(w io.Writer, start, end, step, mode int) {
	fmt.Fprintf(w, "TIME         %5d%5d%5d%5d\n", start, end, step, mode)
}

func writeMonthCard(w io.Writer, year int, month time.Month) {
	fmt.Fprintf(w, "MONTH     %5d%5.2f\n", year, float64(month))
}

func writeSunspotCard(w io.Writer, sunspotNumber int) {
	fmt.Fprintf(w, "SUNSPOT   %5.2f\n", float64(sunspotNumber))
}

func writeLabelCard(w io.Writer, txName, rxName string) {
	fmt.Fprintf(w, "LABEL     %-20s%-20s\n", txName, rxName)
}

func writeCircuitCard(w io.Writer, txLat, txLon, rxLat, rxLon float64, path int) {
	fmt.Fprintf(w, "CIRCUIT   %5.2f%s%9.2f%s%9.2f%s%9.2f%s  S %5d\n",
		math.Abs(txLat), getLatHemi(txLat),
		math.Abs(txLon), getLonHemi(txLon),
		math.Abs(rxLat), getLatHemi(rxLat),
		math.Abs(rxLon), getLonHemi(rxLon),
		path)
}

func writeSystemCard(w io.Writer, requiredSnr float64) {
	fmt.Fprintf(w, "SYSTEM    %5.2f%5.0f%5.2f%5.0f%5.2f%5.2f%5.2f\n", 1.0, 145.0, 0.10, 90.0, requiredSnr, 3.00, 0.10)
}

func writeFprobCard(w io.Writer) {
	fmt.Fprintf(w, "FPROB     %5.2f%5.2f%5.2f%5.2f\n", 1.0, 1.0, 1.0, 0.0)
}

func writeAntennaCard(w io.Writer, antType, id, minFreq, maxFreq int, freqMHz float64, antFile string, bearing, powerW float64) {
	powerKW := powerW / 1000.0
	fmt.Fprintf(w, "ANTENNA   %5d%5d%5d%5d%10.3f[%-21s]%5.1f%10.4f\n", antType, id, minFreq, maxFreq, freqMHz, antFile, bearing, powerKW)
}

func writeFrequencyCard(w io.Writer, freqMHz float64) {
	fmt.Fprintf(w, "FREQUENCY %5.3f\n", freqMHz)
}

func writeMethodCard(w io.Writer, method, spec int) {
	fmt.Fprintf(w, "METHOD    %5d%5d\n", method, spec)
}

func writeExecuteCard(w io.Writer) {
	fmt.Fprintln(w, "EXECUTE")
}

func writeQuitCard(w io.Writer) {
	fmt.Fprintln(w, "QUIT")
}
