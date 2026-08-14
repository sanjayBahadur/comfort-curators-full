package main

import (
	"encoding/xml"
	"fmt"
	"time"
)

type JUnitTestSuites struct {
	XMLName xml.Name     `xml:"testsuites"`
	Suites  []JUnitSuite `xml:"testsuite"`
}

type JUnitSuite struct {
	Name       string      `xml:"name,attr"`
	Tests      int         `xml:"tests,attr"`
	Failures   int         `xml:"failures,attr"`
	Errors     int         `xml:"errors,attr"`
	Skipped    int         `xml:"skipped,attr"`
	Time       string      `xml:"time,attr"`
	Properties *JUnitProps `xml:"properties,omitempty"`
	Cases      []JUnitCase `xml:"testcase"`
	Stdout     string      `xml:"system-out,omitempty"`
	Stderr     string      `xml:"system-err,omitempty"`
}

type JUnitProps struct {
	Props []JUnitProp `xml:"property"`
}

type JUnitProp struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type JUnitCase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *JUnitFailure `xml:"failure,omitempty"`
	Error     *JUnitError   `xml:"error,omitempty"`
	Skipped   *JUnitSkipped `xml:"skipped,omitempty"`
	SystemOut string        `xml:"system-out,omitempty"`
}

type JUnitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Text    string `xml:",chardata"`
}

type JUnitError struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Text    string `xml:",chardata"`
}

type JUnitSkipped struct {
	Message string `xml:"message,attr"`
}

func generateJUnit(phase int, results []ProbeResult, start time.Time, props []JUnitProp) *JUnitTestSuites {
	elapsed := fmt.Sprintf("%.3f", time.Since(start).Seconds())

	failures := 0
	errors := 0
	skipped := 0
	cases := make([]JUnitCase, 0, len(results))

	for _, r := range results {
		var tc JUnitCase
		tc.Name = r.Name
		tc.Classname = r.Group
		tc.Time = fmt.Sprintf("%.3f", r.Duration.Seconds())
		tc.SystemOut = r.Output

		switch r.Status {
		case "pass":
		case "fail":
			failures++
			tc.Failure = &JUnitFailure{
				Message: r.Error,
				Type:    "assertion",
				Text:    r.Error,
			}
		case "error":
			errors++
			tc.Error = &JUnitError{
				Message: r.Error,
				Type:    "runtime",
				Text:    r.Error,
			}
		case "skip":
			skipped++
			tc.Skipped = &JUnitSkipped{
				Message: r.Error,
			}
		}
		cases = append(cases, tc)
	}

	var suiteProps *JUnitProps
	if len(props) > 0 {
		suiteProps = &JUnitProps{Props: props}
	}

	suite := JUnitSuite{
		Name:       fmt.Sprintf("acceptance-phase-%d", phase),
		Tests:      len(results),
		Failures:   failures,
		Errors:     errors,
		Skipped:    skipped,
		Time:       elapsed,
		Properties: suiteProps,
		Cases:      cases,
	}

	return &JUnitTestSuites{
		Suites: []JUnitSuite{suite},
	}
}

func writeJUnit(suites *JUnitTestSuites) error {
	out, err := xml.MarshalIndent(suites, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JUnit: %w", err)
	}
	fmt.Printf("%s%s\n", xml.Header, out)
	return nil
}
