package evaluation

import "encoding/json"

type Result string

const (
	ResultPass Result = "pass"
	ResultFail Result = "fail"
)

type Category string

const (
	CatDenial     Category = "denial"
	CatEscalation Category = "escalation"
	CatInjection  Category = "injection"
	CatFailure    Category = "failure"
)

type Engine string

const (
	EngineJarvis Engine = "jarvis"
	EngineHermes Engine = "hermes"
)

type Scenario struct {
	Name        string
	Description string
	Category    Category
	Engine      Engine
	Evaluate    func() ScenarioResult
}

type ScenarioResult struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    Category `json:"category"`
	Engine      Engine   `json:"engine"`
	Result      Result   `json:"result"`
	Reason      string   `json:"reason,omitempty"`
}

type Score struct {
	Total           int `json:"total"`
	Passed          int `json:"passed"`
	Failed          int `json:"failed"`
	DenialScore     int `json:"denial_score"`
	EscalationScore int `json:"escalation_score"`
	InjectionScore  int `json:"injection_score"`
	FailureScore    int `json:"failure_score"`
}

type Report struct {
	Scenarios []ScenarioResult `json:"scenarios"`
	Score     Score            `json:"score"`
	Passed    bool             `json:"passed"`
}

func (r *Report) JSON() json.RawMessage {
	b, _ := json.Marshal(r)
	return json.RawMessage(b)
}
