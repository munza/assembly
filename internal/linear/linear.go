// Package linear polls the Linear GraphQL API for issue changes.
package linear

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const apiURL = "https://api.linear.app/graphql"

// Issue is the subset foreman cares about.
type Issue struct {
	ID     string `json:"id"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	Team   struct {
		Key string `json:"key"`
	} `json:"team"`
	State struct {
		Name string `json:"name"`
	} `json:"state"`
	Assignee struct {
		Name string `json:"name"`
	} `json:"assignee"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Ref is "TEAM-123" for an issue.
func (i Issue) Ref() string {
	if i.Team.Key == "" {
		return fmt.Sprintf("#%d", i.Number)
	}
	return fmt.Sprintf("%s-%d", i.Team.Key, i.Number)
}

type issuesResponse struct {
	Data struct {
		Issues struct {
			Nodes []Issue `json:"nodes"`
		} `json:"issues"`
		Errors []struct{ Message string } `json:"errors"`
	} `json:"data"`
}

// Client talks to Linear with an API key.
type Client struct{ Key string }

// Enabled reports whether the client has credentials.
func (c Client) Enabled() bool { return c.Key != "" }

// IssuesUpdatedSince returns issues with updatedAt >= since (team filter optional).
func (c Client) IssuesUpdatedSince(since time.Time, teamID string) ([]Issue, error) {
	query := `query($since: DateTime!, $team: String) {
		issues(filter: { updatedAt: { gte: $since }, team: { key: { eq: $team } } }, first: 50, orderBy: updatedAt) {
			nodes {
				id number title updatedAt
				team { key }
				state { name }
				assignee { name }
			}
		}
	}`
	vars := map[string]any{"since": since.Format(time.RFC3339), "team": teamID}
	var resp issuesResponse
	if err := c.do(query, vars, &resp); err != nil {
		return nil, err
	}
	if len(resp.Data.Errors) > 0 {
		return nil, fmt.Errorf("linear: %s", resp.Data.Errors[0].Message)
	}
	return resp.Data.Issues.Nodes, nil
}

func (c Client) do(query string, vars map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.Key)
	req.Header.Set("Content-Type", "application/json")
	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("linear: http %d", httpResp.StatusCode)
	}
	return json.NewDecoder(httpResp.Body).Decode(out)
}
