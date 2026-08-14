package evaluation

import (
	"fmt"
	"sort"
)

type Runner struct {
	scenarios []Scenario
}

func NewRunner() *Runner {
	return &Runner{
		scenarios: AllScenarios(),
	}
}

func NewSuperhostRunner() *Runner {
	return &Runner{
		scenarios: SuperhostScenarios(),
	}
}

func NewHermesRunner() *Runner {
	return &Runner{
		scenarios: HermesScenarios(),
	}
}

func (r *Runner) Run() Report {
	results := make([]ScenarioResult, 0, len(r.scenarios))
	score := Score{Total: len(r.scenarios)}

	for _, s := range r.scenarios {
		sr := s.Evaluate()
		sr.Name = s.Name
		sr.Description = s.Description
		sr.Category = s.Category
		sr.Engine = s.Engine
		if sr.Reason == "" && sr.Result == ResultFail {
			sr.Reason = "expected pass but got fail"
		}
		results = append(results, sr)

		if sr.Result == ResultPass {
			score.Passed++
			switch sr.Category {
			case CatDenial:
				score.DenialScore++
			case CatEscalation:
				score.EscalationScore++
			case CatInjection:
				score.InjectionScore++
			case CatFailure:
				score.FailureScore++
			}
		} else {
			score.Failed++
		}
	}

	passed := score.Failed == 0

	return Report{
		Scenarios: results,
		Score:     score,
		Passed:    passed,
	}
}

func (r *Runner) RunByEngine(engine Engine) Report {
	var filtered []Scenario
	for _, s := range r.scenarios {
		if s.Engine == engine {
			filtered = append(filtered, s)
		}
	}
	runner := &Runner{scenarios: filtered}
	return runner.Run()
}

type Verifier func(report Report) []error

func VerifyAcceptanceCriteria(report Report) []error {
	var errs []error

	if report.Score.DenialScore == 0 {
		errs = append(errs, fmt.Errorf("acceptance: no denial scenarios scored (have %d)", report.Score.DenialScore))
	}
	if report.Score.EscalationScore == 0 {
		errs = append(errs, fmt.Errorf("acceptance: no escalation scenarios scored (have %d)", report.Score.EscalationScore))
	}
	if report.Score.InjectionScore == 0 {
		errs = append(errs, fmt.Errorf("acceptance: no injection scenarios scored (have %d)", report.Score.InjectionScore))
	}
	if report.Score.FailureScore == 0 {
		errs = append(errs, fmt.Errorf("acceptance: no failure scenarios scored (have %d)", report.Score.FailureScore))
	}

	if !report.Passed {
		failures := sortedFailureNames(report.Scenarios)
		errs = append(errs, fmt.Errorf("acceptance: %d scenario(s) failed: %v", report.Score.Failed, failures))
	}

	return errs
}

func sortedFailureNames(results []ScenarioResult) []string {
	var fails []string
	for _, sr := range results {
		if sr.Result == ResultFail {
			fails = append(fails, sr.Name)
		}
	}
	sort.Strings(fails)
	return fails
}

func SortedResults(report Report) []ScenarioResult {
	sorted := make([]ScenarioResult, len(report.Scenarios))
	copy(sorted, report.Scenarios)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Engine != sorted[j].Engine {
			return sorted[i].Engine < sorted[j].Engine
		}
		if sorted[i].Category != sorted[j].Category {
			return sorted[i].Category < sorted[j].Category
		}
		return sorted[i].Name < sorted[j].Name
	})
	return sorted
}
