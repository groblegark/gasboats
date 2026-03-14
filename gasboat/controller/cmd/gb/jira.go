package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var jiraCmd = &cobra.Command{
	Use:     "jira",
	Short:   "JIRA ticket commands",
	Long:    `Fetch and display JIRA tickets. Requires JIRA_BASE_URL, JIRA_EMAIL, and JIRA_API_TOKEN env vars.`,
	GroupID: "orchestration",
}

var jiraShowCmd = &cobra.Command{
	Use:   "show <ticket-id>",
	Short: "Fetch and display a JIRA ticket",
	Long: `Fetches a JIRA ticket via REST API v3 and outputs a structured summary
including title, description, status, assignee, labels, linked issues, and comments.

Requires environment variables:
  JIRA_BASE_URL   - e.g. https://pihealth.atlassian.net
  JIRA_EMAIL      - JIRA account email
  JIRA_API_TOKEN  - JIRA API token`,
	Args: cobra.ExactArgs(1),
	RunE: runJiraShow,
}

func runJiraShow(cmd *cobra.Command, args []string) error {
	ticketID := strings.ToUpper(strings.TrimSpace(args[0]))

	baseURL := os.Getenv("JIRA_BASE_URL")
	email := os.Getenv("JIRA_EMAIL")
	apiToken := os.Getenv("JIRA_API_TOKEN")

	if baseURL == "" || email == "" || apiToken == "" {
		return fmt.Errorf("JIRA_BASE_URL, JIRA_EMAIL, and JIRA_API_TOKEN must all be set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := &jiraHTTPClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		authHeader: "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+apiToken)),
		http:       &http.Client{Timeout: 30 * time.Second},
	}

	// Fetch the issue.
	issue, err := client.getIssue(ctx, ticketID)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", ticketID, err)
	}

	// Fetch comments.
	maxComments, _ := cmd.Flags().GetInt("max-comments")
	comments, err := client.getComments(ctx, ticketID, maxComments)
	if err != nil {
		// Non-fatal — show issue without comments.
		comments = nil
	}

	if jsonOutput {
		out := map[string]any{
			"key":    issue.Key,
			"fields": issue.Fields,
		}
		if comments != nil {
			out["comments"] = comments
		}
		printJSON(out)
		return nil
	}

	printJiraTicket(issue, comments)
	return nil
}

// ── JIRA HTTP client ───────────────────────────────────────────────

type jiraHTTPClient struct {
	baseURL    string
	authHeader string
	http       *http.Client
}

type jiraIssueResponse struct {
	Key    string               `json:"key"`
	ID     string               `json:"id"`
	Fields jiraIssueFieldsResp  `json:"fields"`
}

type jiraIssueFieldsResp struct {
	Summary     string          `json:"summary"`
	Description json.RawMessage `json:"description"`
	Status      *jiraNamedRef   `json:"status"`
	IssueType   *jiraNamedRef   `json:"issuetype"`
	Priority    *jiraNamedRef   `json:"priority"`
	Reporter    *jiraUserRef    `json:"reporter"`
	Assignee    *jiraUserRef    `json:"assignee"`
	Labels      []string        `json:"labels"`
	Parent      *jiraParentRef  `json:"parent"`
	Created     string          `json:"created"`
	Updated     string          `json:"updated"`
	IssueLinks  []jiraLinkRef   `json:"issuelinks"`
}

type jiraNamedRef struct {
	Name string `json:"name"`
}

type jiraUserRef struct {
	DisplayName string `json:"displayName"`
}

type jiraParentRef struct {
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
	} `json:"fields"`
}

type jiraLinkRef struct {
	Type         jiraLinkTypeRef   `json:"type"`
	InwardIssue  *jiraLinkedIssue  `json:"inwardIssue,omitempty"`
	OutwardIssue *jiraLinkedIssue  `json:"outwardIssue,omitempty"`
}

type jiraLinkTypeRef struct {
	Inward  string `json:"inward"`
	Outward string `json:"outward"`
}

type jiraLinkedIssue struct {
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
		Status  *jiraNamedRef `json:"status"`
	} `json:"fields"`
}

type jiraCommentResponse struct {
	Body   json.RawMessage `json:"body"`
	Author *jiraUserRef    `json:"author"`
	Created string         `json:"created"`
}

func (c *jiraHTTPClient) getIssue(ctx context.Context, key string) (*jiraIssueResponse, error) {
	path := "/rest/api/3/issue/" + url.PathEscape(key)
	var issue jiraIssueResponse
	if err := c.doGet(ctx, path, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

func (c *jiraHTTPClient) getComments(ctx context.Context, key string, maxResults int) ([]jiraCommentResponse, error) {
	if maxResults <= 0 {
		maxResults = 20
	}
	path := fmt.Sprintf("/rest/api/3/issue/%s/comment?maxResults=%d&orderBy=-created",
		url.PathEscape(key), maxResults)

	var result struct {
		Comments []jiraCommentResponse `json:"comments"`
		Total    int                   `json:"total"`
	}
	if err := c.doGet(ctx, path, &result); err != nil {
		return nil, err
	}
	return result.Comments, nil
}

func (c *jiraHTTPClient) doGet(ctx context.Context, path string, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		msg := string(body)
		if len(msg) > 256 {
			msg = msg[:256] + "..."
		}
		return fmt.Errorf("JIRA API returned %d: %s", resp.StatusCode, msg)
	}

	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// ── ADF to text ────────────────────────────────────────────────────

func jiraADFToText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var doc struct {
		Content []jiraADFNode `json:"content"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ""
	}

	var sb strings.Builder
	for _, node := range doc.Content {
		jiraRenderNode(&sb, node, 0)
	}
	return strings.TrimSpace(sb.String())
}

type jiraADFNode struct {
	Type    string          `json:"type"`
	Text    string          `json:"text,omitempty"`
	Content []jiraADFNode   `json:"content,omitempty"`
	Attrs   json.RawMessage `json:"attrs,omitempty"`
	Marks   []jiraADFMark   `json:"marks,omitempty"`
}

type jiraADFMark struct {
	Type  string          `json:"type"`
	Attrs json.RawMessage `json:"attrs,omitempty"`
}

func jiraRenderNode(sb *strings.Builder, node jiraADFNode, depth int) {
	switch node.Type {
	case "doc":
		for _, child := range node.Content {
			jiraRenderNode(sb, child, depth)
		}
	case "paragraph":
		jiraRenderInline(sb, node.Content)
		sb.WriteString("\n\n")
	case "heading":
		level := 1
		if len(node.Attrs) > 0 {
			var attrs struct{ Level int `json:"level"` }
			if json.Unmarshal(node.Attrs, &attrs) == nil && attrs.Level > 0 {
				level = attrs.Level
			}
		}
		sb.WriteString(strings.Repeat("#", level) + " ")
		jiraRenderInline(sb, node.Content)
		sb.WriteString("\n\n")
	case "bulletList":
		for _, item := range node.Content {
			if item.Type == "listItem" {
				sb.WriteString("- ")
				for i, child := range item.Content {
					if i > 0 {
						sb.WriteString("  ")
					}
					jiraRenderInline(sb, child.Content)
				}
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")
	case "orderedList":
		for i, item := range node.Content {
			if item.Type == "listItem" {
				fmt.Fprintf(sb, "%d. ", i+1)
				for j, child := range item.Content {
					if j > 0 {
						sb.WriteString("   ")
					}
					jiraRenderInline(sb, child.Content)
				}
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")
	case "codeBlock":
		lang := ""
		if len(node.Attrs) > 0 {
			var attrs struct{ Language string `json:"language"` }
			if json.Unmarshal(node.Attrs, &attrs) == nil {
				lang = attrs.Language
			}
		}
		sb.WriteString("```" + lang + "\n")
		jiraRenderInline(sb, node.Content)
		sb.WriteString("\n```\n\n")
	case "blockquote":
		for _, child := range node.Content {
			sb.WriteString("> ")
			jiraRenderInline(sb, child.Content)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
}

func jiraRenderInline(sb *strings.Builder, nodes []jiraADFNode) {
	for _, n := range nodes {
		switch n.Type {
		case "text":
			text := n.Text
			for _, mark := range n.Marks {
				switch mark.Type {
				case "strong":
					text = "**" + text + "**"
				case "em":
					text = "_" + text + "_"
				case "code":
					text = "`" + text + "`"
				case "link":
					var attrs struct{ Href string `json:"href"` }
					if json.Unmarshal(mark.Attrs, &attrs) == nil && attrs.Href != "" {
						text = "[" + text + "](" + attrs.Href + ")"
					}
				}
			}
			sb.WriteString(text)
		case "hardBreak":
			sb.WriteString("\n")
		case "inlineCard":
			if len(n.Attrs) > 0 {
				var attrs struct{ URL string `json:"url"` }
				if json.Unmarshal(n.Attrs, &attrs) == nil && attrs.URL != "" {
					sb.WriteString(attrs.URL)
				}
			}
		}
	}
}

// ── Output formatting ──────────────────────────────────────────────

func printJiraTicket(issue *jiraIssueResponse, comments []jiraCommentResponse) {
	f := issue.Fields

	fmt.Printf("# %s: %s\n\n", issue.Key, f.Summary)

	fmt.Printf("%-12s %s\n", "Type:", nameOrEmpty(f.IssueType))
	fmt.Printf("%-12s %s\n", "Status:", nameOrEmpty(f.Status))
	fmt.Printf("%-12s %s\n", "Priority:", nameOrEmpty(f.Priority))
	fmt.Printf("%-12s %s\n", "Assignee:", userOrEmpty(f.Assignee))
	fmt.Printf("%-12s %s\n", "Reporter:", userOrEmpty(f.Reporter))
	if len(f.Labels) > 0 {
		fmt.Printf("%-12s %s\n", "Labels:", strings.Join(f.Labels, ", "))
	}
	if f.Parent != nil {
		fmt.Printf("%-12s %s — %s\n", "Parent:", f.Parent.Key, f.Parent.Fields.Summary)
	}
	fmt.Printf("%-12s %s\n", "Created:", formatJiraTime(f.Created))
	fmt.Printf("%-12s %s\n", "Updated:", formatJiraTime(f.Updated))

	// Linked issues.
	if len(f.IssueLinks) > 0 {
		fmt.Printf("\n## Linked Issues\n\n")
		for _, link := range f.IssueLinks {
			if link.OutwardIssue != nil {
				status := ""
				if link.OutwardIssue.Fields.Status != nil {
					status = " [" + link.OutwardIssue.Fields.Status.Name + "]"
				}
				fmt.Printf("  %s %s — %s%s\n", link.Type.Outward, link.OutwardIssue.Key,
					link.OutwardIssue.Fields.Summary, status)
			}
			if link.InwardIssue != nil {
				status := ""
				if link.InwardIssue.Fields.Status != nil {
					status = " [" + link.InwardIssue.Fields.Status.Name + "]"
				}
				fmt.Printf("  %s %s — %s%s\n", link.Type.Inward, link.InwardIssue.Key,
					link.InwardIssue.Fields.Summary, status)
			}
		}
	}

	// Description.
	desc := jiraADFToText(f.Description)
	if desc != "" {
		fmt.Printf("\n## Description\n\n%s\n", desc)
	} else {
		fmt.Printf("\n## Description\n\n(no description)\n")
	}

	// Comments.
	if len(comments) > 0 {
		fmt.Printf("\n## Comments (%d)\n\n", len(comments))
		for _, c := range comments {
			author := "unknown"
			if c.Author != nil {
				author = c.Author.DisplayName
			}
			body := jiraADFToText(c.Body)
			if len(body) > 500 {
				body = body[:500] + "..."
			}
			fmt.Printf("### %s (%s)\n%s\n\n", author, formatJiraTime(c.Created), body)
		}
	}
}

func nameOrEmpty(ref *jiraNamedRef) string {
	if ref != nil {
		return ref.Name
	}
	return "(none)"
}

func userOrEmpty(ref *jiraUserRef) string {
	if ref != nil {
		return ref.DisplayName
	}
	return "(unassigned)"
}

func formatJiraTime(s string) string {
	t, err := time.Parse("2006-01-02T15:04:05.000-0700", s)
	if err != nil {
		return s
	}
	return t.Format("2006-01-02 15:04")
}

func init() {
	jiraShowCmd.Flags().Int("max-comments", 20, "maximum number of comments to fetch")

	jiraCmd.AddCommand(jiraShowCmd)
}
