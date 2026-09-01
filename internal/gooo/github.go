package gooo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const defaultGitHubAPI = "https://api.github.com"

type githubObserver struct {
	baseURL string
	token   string
	client  *http.Client
}

type rulesetSummary struct {
	ID int `json:"id"`
}

type rulesetDetail struct {
	Target      string `json:"target"`
	Enforcement string `json:"enforcement"`
	Conditions  struct {
		RefName struct {
			Include []string `json:"include"`
		} `json:"ref_name"`
	} `json:"conditions"`
	Rules []struct {
		Type string `json:"type"`
	} `json:"rules"`
}

func ObserveGitHub(ctx context.Context, repo, sha, event, ref, before string) (Observation, error) {
	observer := githubObserver{baseURL: defaultGitHubAPI, token: os.Getenv("GITHUB_TOKEN"), client: http.DefaultClient}
	return observer.observe(ctx, repo, sha, event, ref, before)
}

func (o githubObserver) observe(ctx context.Context, repo, sha, event, ref, before string) (Observation, error) {
	observation := Observation{
		BootstrapCommits:      1,
		BootstrapCommitsKnown: true,
		GitHubAPI:             "observed",
		Ruleset:               "observed",
	}
	if repo == "" || o.token == "" {
		return Observation{BootstrapCommitsKnown: false, PostBootstrapDirectMainKnown: false, GitHubAPI: "insufficient", Ruleset: "insufficient"}, errors.New("GitHub repository or token is unavailable")
	}
	if event == "push" && ref == "refs/heads/main" {
		if isZeroSHA(before) {
			observation.PostBootstrapDirectMain = 0
			observation.PostBootstrapDirectMainKnown = true
		} else {
			pulls, err := o.commitPulls(ctx, repo, sha)
			if err != nil {
				observation.PostBootstrapDirectMainKnown = false
				observation.GitHubAPI = "insufficient"
			} else {
				observation.PostBootstrapDirectMainKnown = true
				if len(pulls) == 0 {
					observation.PostBootstrapDirectMain = 1
				}
			}
		}
	} else {
		observation.PostBootstrapDirectMain = 0
		observation.PostBootstrapDirectMainKnown = true
	}
	if err := o.observeRuleset(ctx, repo, &observation); err != nil {
		return observation, err
	}
	return observation, nil
}

func (o githubObserver) commitPulls(ctx context.Context, repo, sha string) ([]json.RawMessage, error) {
	var response []json.RawMessage
	if err := o.get(ctx, fmt.Sprintf("/repos/%s/commits/%s/pulls", repo, sha), &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (o githubObserver) observeRuleset(ctx context.Context, repo string, observation *Observation) error {
	var summaries []rulesetSummary
	if err := o.get(ctx, fmt.Sprintf("/repos/%s/rulesets?includes_parents=true", repo), &summaries); err != nil {
		observation.Ruleset = "insufficient"
		observation.GitHubAPI = "insufficient"
		return nil
	}
	for _, summary := range summaries {
		var detail rulesetDetail
		if err := o.get(ctx, fmt.Sprintf("/repos/%s/rulesets/%d", repo, summary.ID), &detail); err != nil {
			observation.Ruleset = "insufficient"
			observation.GitHubAPI = "insufficient"
			return nil
		}
		if detail.Target != "branch" || detail.Enforcement != "active" {
			continue
		}
		matchesMain := false
		for _, include := range detail.Conditions.RefName.Include {
			if include == "~DEFAULT_BRANCH" || include == "refs/heads/main" || include == "main" {
				matchesMain = true
			}
		}
		hasPullRequestRule := false
		for _, rule := range detail.Rules {
			if rule.Type == "pull_request" {
				hasPullRequestRule = true
			}
		}
		if matchesMain && hasPullRequestRule {
			observation.Ruleset = "observed"
			return nil
		}
	}
	observation.Ruleset = "insufficient"
	return nil
}

func (o githubObserver) get(ctx context.Context, path string, result any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(o.baseURL, "/")+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("Authorization", "Bearer "+o.token)
	response, err := o.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return fmt.Errorf("GitHub REST GET %s returned %s: %s", path, response.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(response.Body).Decode(result)
}

func isZeroSHA(value string) bool {
	return value != "" && strings.Trim(value, "0") == "" && len(value) == 40
}
