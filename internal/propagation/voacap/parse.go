package voacap

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// VoacapOutput holds the parsed data from a VOACAP output file.
type VoacapOutput struct {
	Title       string
	Version     string
	Request     Request
	Coeffs      string
	Method      string
	Date        string
	SSN         float64
	MinAngle    float64
	Circuit     Circuit
	Transmitter Antenna
	Receiver    Antenna
	Noise       float64
	RequiredRel float64
	RequiredSNR float64
	PowerTol    float64
	DelayTol    float64
	Predictions []Prediction
	MufLuf      []MufLuf
}

// Request holds the input parameters for the VOACAP run.
type Request struct {
	Hour      int
	Frequency float64
}

// MufLuf holds the data from a METHOD 26 prediction table.
type MufLuf struct {
	GMT   float64
	LMT   float64
	FOT   float64
	HPF   float64
	ESMUF float64
	MUF   float64
	LUF   float64
}

// Circuit holds information about the communication path.
type Circuit struct {
	From       Location
	To         Location
	Azimuths   []float64
	DistanceNM float64
	DistanceKM float64
}

// Location holds geographic coordinates and name.
type Location struct {
	Name string
	Lat  string
	Lon  string
}

// Antenna holds antenna information.
type Antenna struct {
	Description string
	Azimuth     float64
	OffAzimuth  float64
	PowerKW     float64
}

// Prediction holds the data for a single prediction table, corresponding to one hour.
type Prediction struct {
	Hour            float64
	BandPredictions []BandPrediction
}

// BandPrediction holds all the predicted values for a single frequency band.
// Field names are derived from the output file (e.g., "V HITE" becomes "VHite").
type BandPrediction struct {
	Freq   float64
	Mode   string
	Tangle float64
	Delay  float64
	VHite  float64
	MUFday float64
	Loss   float64
	DBU    float64
	SDBW   float64
	NDBW   float64
	SNR    float64
	RPWRG  float64
	Rel    float64
	MProb  float64
	SPrb   float64
	SigLw  float64
	SigUp  float64
	SnrLw  float64
	SnrUp  float64
	TGain  float64
	RGain  float64
	SNRxx  float64
}

// Regex definitions for parsing different lines of the VOACAP output.
var (
	reTitle      = regexp.MustCompile(`IONOSPHERIC COMMUNICATIONS ANALYSIS AND PREDICTION PROGRAM`)
	reVersion    = regexp.MustCompile(`VOACAP\s+VERSION\s+([\d.W]+)`)
	reCoeffs     = regexp.MustCompile(`(\w+)\s+Coefficients.*METHOD\s+(\d+)`)
	reSSN        = regexp.MustCompile(`SSN\s*=\s*([\d.-]+)\s*Minimum Angle=\s*([\d.-]+)`)
	reCircuit    = regexp.MustCompile(`(\d+\.\d+)\s*([NS])\s+(\d+\.\d+)\s*([EW])\s+-\s+(\d+\.\d+)\s*([NS])\s+(\d+\.\d+)\s*([EW])\s+([\d.-]+)\s+([\d.-]+)\s+([\d.-]+)\s+([\d.-]+)`)
	reAntennaPwr = regexp.MustCompile(`(XMTR|RCVR)\s+(.*?)\s+Az=\s*([\d.-]+)\s+OFFaz=\s*([\d.-]+)\s+([\d.-]+)kW`)
	reAntenna    = regexp.MustCompile(`(XMTR|RCVR)\s+(.*?)\s+Az=\s*([\d.-]+)\s+OFFaz=\s*([\d.-]+)`)
	reAntennaIn  = regexp.MustCompile(`\[(.*?)\]`)
	reNoise      = regexp.MustCompile(`NOISE\s*=\s*([-\d.]+)\s*dBW\s*REQ\. REL\s*=\s*(\d+)\%\s*REQ\. SNR\s*=\s*([-\d.]+)`)
	reMultipath  = regexp.MustCompile(`POWER TOLERANCE\s*=\s*([\d.]+)\s*dB\s*MULTIPATH DELAY TOLERANCE\s*=\s*([\d.]+)\s*ms`)
	reMethod26   = regexp.MustCompile(`GMT\s+LMT\s+FOT\s+HPF\s+ESMUF\s+MUF\s+LUF`)
	reTime       = regexp.MustCompile(`TIME\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)`)
	reFrequency  = regexp.MustCompile(`FREQUENCY\s+([\d.]+)`)
)

func Parse(r io.Reader) (*VoacapOutput, error) {
	scanner := bufio.NewScanner(r)
	return parse(scanner)
}

func parse(scanner *bufio.Scanner) (*VoacapOutput, error) {
	output := &VoacapOutput{}
	var predictions []Prediction
	var currentPrediction *Prediction

scanLoop:
	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case reTitle.MatchString(line):
			output.Title = strings.TrimSpace(line)
		case reVersion.MatchString(line):
			if err := parseVersion(line, output); err != nil {
				return nil, fmt.Errorf("parsing version: %w", err)
			}
		case reCoeffs.MatchString(line):
			if err := parseCoeffs(line, output); err != nil {
				return nil, fmt.Errorf("parsing coeffs: %w", err)
			}
		case strings.Contains(line, "SSN ="):
			if err := parseSSN(line, output); err != nil {
				return nil, fmt.Errorf("parsing SSN: %w", err)
			}
		case reTime.MatchString(line):
			if err := parseTime(line, output); err != nil {
				return nil, fmt.Errorf("parsing time: %w", err)
			}
		case reFrequency.MatchString(line):
			if err := parseFrequency(line, output); err != nil {
				return nil, fmt.Errorf("parsing frequency: %w", err)
			}
		case reCircuit.MatchString(line):
			if err := parseCircuit(line, output); err != nil {
				return nil, fmt.Errorf("parsing circuit: %w", err)
			}
		case strings.Contains(line, "ANTENNA"):
			if strings.Contains(line, "XMTR") {
				if err := parseAntennaInput(line, &output.Transmitter); err != nil {
					return nil, fmt.Errorf("parsing transmitter antenna input: %w", err)
				}
			} else if strings.Contains(line, "RCVR") {
				if err := parseAntennaInput(line, &output.Receiver); err != nil {
					return nil, fmt.Errorf("parsing receiver antenna input: %w", err)
				}
			}
		case strings.Contains(line, "XMTR"):
			ant, err := parseAntenna(line)
			if err != nil {
				return nil, fmt.Errorf("parsing transmitter antenna: %w", err)
			}
			output.Transmitter = ant
		case strings.Contains(line, "RCVR"):
			ant, err := parseAntenna(line)
			if err != nil {
				return nil, fmt.Errorf("parsing receiver antenna: %w", err)
			}
			output.Receiver = ant
		case reNoise.MatchString(line):
			if err := parseNoise(line, output); err != nil {
				return nil, fmt.Errorf("parsing noise: %w", err)
			}
		case reMultipath.MatchString(line):
			if err := parseMultipath(line, output); err != nil {
				return nil, fmt.Errorf("parsing multipath: %w", err)
			}
		case reMethod26.MatchString(line):
			mufLuf, err := parseMethod26(scanner)
			if err != nil {
				return nil, fmt.Errorf("parsing method 26: %w", err)
			}
			output.MufLuf = mufLuf
		case strings.HasSuffix(strings.TrimSpace(line), "FREQ"):
			if currentPrediction != nil {
				predictions = append(predictions, *currentPrediction)
			}

			// Use fixed-width parsing for the FREQ line
			fields := parseFixedWidthLine(line)
			if len(fields) < 3 { // Need at least hour, one frequency, and "FREQ"
				return nil, fmt.Errorf("not enough fields in prediction header: '%s'", line)
			}

			hour, err := strconv.ParseFloat(fields[0], 64)
			if err != nil {
				return nil, fmt.Errorf("parsing prediction hour: %w", err)
			}

			currentPrediction = &Prediction{Hour: hour}

			// Make a BandPrediction enty for each frequency
			for _, freqStr := range fields[1 : len(fields)-1] { // first field is hour and last is FREQ
				if freqStr == "" {
					continue
				}
				freq, err := parseFloat(freqStr)
				if err != nil {
					return nil, fmt.Errorf("parsing frequency: %w", err)
				}
				currentPrediction.BandPredictions = append(currentPrediction.BandPredictions, BandPrediction{Freq: freq})
			}

		case currentPrediction != nil && strings.TrimSpace(line) != "" && !strings.Contains(line, "*****END OF RUN*****"):
			// Use fixed-width parsing for data lines
			paramFields := parseFixedWidthLine(line)
			if len(paramFields) < 2 {
				continue
			}

			// The last field is the parameter name
			paramName := paramFields[len(paramFields)-1]

			// The parameter values are all fields except the last (parameter name)
			// and skip the first field which is always empty or contains irrelevant data
			paramValues := paramFields[1 : len(paramFields)-1]

			if len(paramValues) != len(currentPrediction.BandPredictions) {
				// This can happen for lines that are not part of the table, like the empty ones.
				continue
			}

			for i, v := range paramValues {
				var err error
				switch paramName {
				case "MODE":
					currentPrediction.BandPredictions[i].Mode = v
				case "TANGLE":
					currentPrediction.BandPredictions[i].Tangle, err = parseFloat(v)
				case "DELAY":
					currentPrediction.BandPredictions[i].Delay, err = parseFloat(v)
				case "V HITE":
					currentPrediction.BandPredictions[i].VHite, err = parseFloat(v)
				case "MUFday":
					currentPrediction.BandPredictions[i].MUFday, err = parseFloat(v)
				case "LOSS":
					currentPrediction.BandPredictions[i].Loss, err = parseFloat(v)
				case "DBU":
					currentPrediction.BandPredictions[i].DBU, err = parseFloat(v)
				case "S DBW":
					currentPrediction.BandPredictions[i].SDBW, err = parseFloat(v)
				case "N DBW":
					currentPrediction.BandPredictions[i].NDBW, err = parseFloat(v)
				case "SNR":
					currentPrediction.BandPredictions[i].SNR, err = parseFloat(v)
				case "RPWRG":
					currentPrediction.BandPredictions[i].RPWRG, err = parseFloat(v)
				case "REL":
					currentPrediction.BandPredictions[i].Rel, err = parseFloat(v)
				case "MPROB":
					currentPrediction.BandPredictions[i].MProb, err = parseFloat(v)
				case "S PRB":
					currentPrediction.BandPredictions[i].SPrb, err = parseFloat(v)
				case "SIG LW":
					currentPrediction.BandPredictions[i].SigLw, err = parseFloat(v)
				case "SIG UP":
					currentPrediction.BandPredictions[i].SigUp, err = parseFloat(v)
				case "SNR LW":
					currentPrediction.BandPredictions[i].SnrLw, err = parseFloat(v)
				case "SNR UP":
					currentPrediction.BandPredictions[i].SnrUp, err = parseFloat(v)
				case "TGAIN":
					currentPrediction.BandPredictions[i].TGain, err = parseFloat(v)
				case "RGAIN":
					currentPrediction.BandPredictions[i].RGain, err = parseFloat(v)
				case "SNRxx":
					currentPrediction.BandPredictions[i].SNRxx, err = parseFloat(v)
				}
				if err != nil {
					return nil, fmt.Errorf("parsing param %s value '%s': %w", paramName, v, err)
				}
			}
		case strings.Contains(line, "*****END OF RUN*****"):
			if currentPrediction != nil {
				predictions = append(predictions, *currentPrediction)
			}
			break scanLoop
		}
	}

	output.Predictions = predictions
	return output, scanner.Err()
}

func parseVersion(line string, out *VoacapOutput) error {
	matches := reVersion.FindStringSubmatch(line)
	if len(matches) < 2 {
		return fmt.Errorf("could not find version in line: %s", line)
	}
	out.Version = matches[1]
	return nil
}

func parseCoeffs(line string, out *VoacapOutput) error {
	matches := reCoeffs.FindStringSubmatch(line)
	if len(matches) < 3 {
		return fmt.Errorf("could not find coeffs and method in line: %s", line)
	}
	out.Coeffs = matches[1]
	out.Method = matches[2]
	return nil
}

func parseSSN(line string, out *VoacapOutput) error {
	matches := reSSN.FindStringSubmatch(line)
	if len(matches) < 3 {
		return fmt.Errorf("could not find SSN and Minimum Angle in line: %s", line)
	}
	var err error
	out.SSN, err = strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return fmt.Errorf("parsing SSN value: %w", err)
	}
	out.MinAngle, err = strconv.ParseFloat(matches[2], 64)
	if err != nil {
		return fmt.Errorf("parsing Minimum Angle value: %w", err)
	}
	out.Date = strings.TrimSpace(line[:strings.Index(line, "SSN =")])
	return nil
}

func parseCircuit(line string, out *VoacapOutput) error {
	matches := reCircuit.FindStringSubmatch(line)
	if len(matches) < 13 {
		return fmt.Errorf("could not parse circuit line: %s", line)
	}
	out.Circuit.From.Lat = matches[1] + matches[2]
	out.Circuit.From.Lon = matches[3] + matches[4]
	out.Circuit.To.Lat = matches[5] + matches[6]
	out.Circuit.To.Lon = matches[7] + matches[8]

	az1, err := strconv.ParseFloat(matches[9], 64)
	if err != nil {
		return fmt.Errorf("parsing azimuth 1: %w", err)
	}
	az2, err := strconv.ParseFloat(matches[10], 64)
	if err != nil {
		return fmt.Errorf("parsing azimuth 2: %w", err)
	}
	out.Circuit.Azimuths = []float64{az1, az2}

	out.Circuit.DistanceNM, err = strconv.ParseFloat(matches[11], 64)
	if err != nil {
		return fmt.Errorf("parsing distance NM: %w", err)
	}
	out.Circuit.DistanceKM, err = strconv.ParseFloat(matches[12], 64)
	if err != nil {
		return fmt.Errorf("parsing distance KM: %w", err)
	}
	return nil
}

func parseAntenna(line string) (Antenna, error) {
	var matches []string
	if reAntennaPwr.MatchString(line) {
		matches = reAntennaPwr.FindStringSubmatch(line)
	} else {
		matches = reAntenna.FindStringSubmatch(line)
	}

	if len(matches) < 5 {
		return Antenna{}, fmt.Errorf("could not parse antenna line: %s", line)
	}

	desc := strings.TrimSpace(matches[2])
	if reAntennaIn.MatchString(desc) {
		desc = strings.TrimSpace(reAntennaIn.FindStringSubmatch(desc)[1])
	}

	ant := Antenna{
		Description: desc,
	}

	var err error
	ant.Azimuth, err = strconv.ParseFloat(matches[3], 64)
	if err != nil {
		return Antenna{}, fmt.Errorf("parsing antenna azimuth: %w", err)
	}
	ant.OffAzimuth, err = strconv.ParseFloat(matches[4], 64)
	if err != nil {
		return Antenna{}, fmt.Errorf("parsing antenna off-azimuth: %w", err)
	}

	if len(matches) > 5 {
		ant.PowerKW, err = strconv.ParseFloat(matches[5], 64)
		if err != nil {
			return Antenna{}, fmt.Errorf("parsing antenna power: %w", err)
		}
	}

	return ant, nil
}

func parseNoise(line string, out *VoacapOutput) error {
	matches := reNoise.FindStringSubmatch(line)
	if len(matches) < 4 {
		return fmt.Errorf("could not parse noise line: %s", line)
	}
	var err error
	out.Noise, err = strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return fmt.Errorf("parsing noise value: %w", err)
	}
	out.RequiredRel, err = strconv.ParseFloat(matches[2], 64)
	if err != nil {
		return fmt.Errorf("parsing required reliability: %w", err)
	}
	out.RequiredSNR, err = strconv.ParseFloat(matches[3], 64)
	if err != nil {
		return fmt.Errorf("parsing required SNR: %w", err)
	}
	return nil
}

func parseMultipath(line string, out *VoacapOutput) error {
	matches := reMultipath.FindStringSubmatch(line)
	if len(matches) < 3 {
		return fmt.Errorf("could not parse multipath line: %s", line)
	}
	var err error
	out.PowerTol, err = strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return fmt.Errorf("parsing power tolerance: %w", err)
	}
	out.DelayTol, err = strconv.ParseFloat(matches[2], 64)
	if err != nil {
		return fmt.Errorf("parsing delay tolerance: %w", err)
	}
	return nil
}

func parseTime(line string, out *VoacapOutput) error {
	matches := reTime.FindStringSubmatch(line)
	if len(matches) < 2 {
		return fmt.Errorf("could not parse time line: %s", line)
	}
	hour, err := strconv.Atoi(matches[1])
	if err != nil {
		return fmt.Errorf("parsing hour: %w", err)
	}
	out.Request.Hour = hour
	return nil
}

func parseFrequency(line string, out *VoacapOutput) error {
	matches := reFrequency.FindStringSubmatch(line)
	if len(matches) < 2 {
		return fmt.Errorf("could not parse frequency line: %s", line)
	}
	freq, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return fmt.Errorf("parsing frequency: %w", err)
	}
	out.Request.Frequency = freq
	return nil
}

func parseFloat(s string) (float64, error) {
	if s == "-" || s == "nan" {
		return math.NaN(), nil
	}
	return strconv.ParseFloat(s, 64)
}

func parseMethod26(scanner *bufio.Scanner) ([]MufLuf, error) {
	var mufLuf []MufLuf
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			return mufLuf, nil
		}
		if strings.Contains(line, "CCIR Coefficients") {
			return mufLuf, nil
		}
		fields := strings.Fields(line)
		if len(fields) != 7 {
			continue
		}
		gmt, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			continue
		}
		lmt, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			return nil, err
		}
		fot, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			return nil, err
		}
		hpf, err := strconv.ParseFloat(fields[3], 64)
		if err != nil {
			return nil, err
		}
		esmuf, err := strconv.ParseFloat(fields[4], 64)
		if err != nil {
			return nil, err
		}
		muf, err := strconv.ParseFloat(fields[5], 64)
		if err != nil {
			return nil, err
		}
		luf, err := strconv.ParseFloat(fields[6], 64)
		if err != nil {
			return nil, err
		}
		mufLuf = append(mufLuf, MufLuf{
			GMT:   gmt,
			LMT:   lmt,
			FOT:   fot,
			HPF:   hpf,
			ESMUF: esmuf,
			MUF:   muf,
			LUF:   luf,
		})
	}
	return mufLuf, scanner.Err()
}

func parseAntennaInput(line string, ant *Antenna) error {
	matches := reAntennaIn.FindStringSubmatch(line)
	if len(matches) < 2 {
		return fmt.Errorf("could not parse antenna input line: %s", line)
	}
	ant.Description = strings.TrimSpace(matches[1])
	return nil
}

// parseFixedWidthLine parses a line of fixed-width fields from VOACAP prediction tables.
// It returns a slice of strings containing the fields.
func parseFixedWidthLine(line string) []string {
	// 1. Remove the first space (indent) from each line
	if len(line) > 0 && line[0] == ' ' {
		line = line[1:]
	}

	// 2. Split by fixed width of 5 for the first 13 fields
	const fieldWidth = 5
	numDataFields := 13
	pos := 0
	var fields []string

	for i := 0; i < numDataFields; i++ {
		endPos := pos + fieldWidth
		if endPos > len(line) {
			endPos = len(line)
		}

		if pos < len(line) {
			field := strings.TrimSpace(line[pos:endPos])
			fields = append(fields, field)
		}
		pos += fieldWidth
	}

	// 3. Take the rest of the line and add it as the label
	if pos < len(line) {
		lastField := strings.TrimSpace(line[pos:])
		fields = append(fields, lastField)
	}

	return fields
}
