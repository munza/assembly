package issue

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

type Issue struct {
	Identifier  string   `json:"identifier"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	URL         string   `json:"url"`
	State       string   `json:"state"`
	Assignee    string   `json:"assignee"`
	Labels      []string `json:"labels"`
}

const issueQuery = `query($id: String!) { issue(id: $id) { identifier title description url state { name } assignee { name } labels { nodes { name } } } }`

type wireIssue struct {
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	State       struct {
		Name string `json:"name"`
	} `json:"state"`
	Assignee struct {
		Name string `json:"name"`
	} `json:"assignee"`
	Labels struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
}

func GetIssue(id, apiKey string) (*Issue, error) {
	key := apiKey
	if key == "" {
		key = os.Getenv("LINEAR_API_KEY")
	}
	if key == "" {
		return nil, errors.New("no Linear API key: set linear.api_key in .assembly/settings.json or LINEAR_API_KEY")
	}
	body, err := json.Marshal(map[string]any{"query": issueQuery, "variables": map[string]any{"id": id}})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", "https://api.linear.app/graphql", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", key)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var out struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Issue *wireIssue `json:"issue"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("linear: %s", out.Errors[0].Message)
	}
	if out.Data.Issue == nil {
		return nil, fmt.Errorf("linear issue %q not found", id)
	}
	w := out.Data.Issue
	issue := &Issue{
		Identifier:  w.Identifier,
		Title:       w.Title,
		Description: w.Description,
		URL:         w.URL,
		State:       w.State.Name,
		Assignee:    w.Assignee.Name,
	}
	for _, n := range w.Labels.Nodes {
		issue.Labels = append(issue.Labels, n.Name)
	}
	return issue, nil
}
