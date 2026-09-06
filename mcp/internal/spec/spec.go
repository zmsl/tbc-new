// Package spec holds the self-describing objects the server is built from.
//
// A tool, resource or prompt is declared once, as data: what it is called, what it is for, what
// it takes, what it returns, and the function that serves it. Everything else is derived from
// that declaration -- the MCP registration in register.go, the JSON Schema an agent validates
// against, and the reference under docs/. Nothing in this package knows what the sim is; nothing
// outside it talks to the MCP SDK.
//
// The point of the indirection is that the description an agent reads and the schema its
// arguments are checked against cannot drift apart: they come from the same declaration, and a
// declaration that fails to describe itself does not register (see Validate).
package spec

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Kind distinguishes the three things a server can expose.
type Kind string

const (
	KindTool     Kind = "tool"
	KindResource Kind = "resource"
	KindPrompt   Kind = "prompt"
)

// Example is a worked call. Examples are part of the description an agent reads, because the
// shape of a request is much easier to copy than to infer from a schema.
type Example struct {
	// Description of what this example achieves, e.g. "compare two trinkets over a long fight".
	Description string
	// Args is the JSON arguments object, as it would be sent.
	Args string
}

// Entry is one registered thing. The interface is deliberately narrow and its registration
// method unexported: an entry must be a Tool, Resource or Prompt declared in this package, so
// every entry is guaranteed to carry documentation and to be validated before it goes live.
type Entry interface {
	Kind() Kind
	// ID is the name a tool or prompt is called by, or the URI a resource is read at.
	ID() string
	// Doc is everything needed to render the entry, for agents and for humans.
	Doc() (Doc, error)
	// Validate reports whether the declaration is complete enough to register.
	Validate() error

	register(server *mcp.Server) error
}

// Doc is the rendered form of an entry: what it is, what it takes, what it gives back.
type Doc struct {
	Kind         Kind
	ID           string
	Title        string
	Description  string
	InputSchema  *jsonschema.Schema
	OutputSchema *jsonschema.Schema
	Arguments    []Argument
	MIMEType     string
	Annotations  *mcp.ToolAnnotations
	// Text is a prompt's rendered workflow, with no arguments supplied. Prompts are templates, so
	// this is what one looks like before it is filled in -- which is what the documentation and
	// the bundle manifest both need to show.
	Text string
}

// Argument is a prompt argument.
type Argument struct {
	Name        string
	Description string
	Required    bool
}

// Tool names are snake_case so they read consistently in a tool list next to other servers'.
var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Tool is a callable with typed input and output. In and Out drive the JSON Schema the SDK
// generates and validates against, so their fields carry `json` and `jsonschema` tags.
type Tool[In, Out any] struct {
	// Name the agent calls, snake_case.
	Name string
	// Title is a short human-readable label.
	Title string
	// Summary is one line saying what the tool does. Required.
	Summary string
	// Details is optional prose: when to reach for it, what it costs, what to watch out for.
	Details string
	// Examples are worked calls, rendered into the description.
	Examples []Example

	// ReadOnly declares that the tool does not change anything. Nearly everything here is.
	ReadOnly bool
	// Idempotent declares that repeating a call has no additional effect. Meaningful only for
	// tools that are not read-only.
	Idempotent bool
	// OpenWorld declares that the tool reaches outside this server. Nothing here does.
	OpenWorld bool

	Handler mcp.ToolHandlerFor[In, Out]
}

func (t Tool[In, Out]) Kind() Kind { return KindTool }
func (t Tool[In, Out]) ID() string { return t.Name }

func (t Tool[In, Out]) Validate() error {
	if !toolNamePattern.MatchString(t.Name) {
		return fmt.Errorf("tool name %q must be snake_case", t.Name)
	}
	if strings.TrimSpace(t.Summary) == "" {
		return fmt.Errorf("tool %q has no summary", t.Name)
	}
	if t.Handler == nil {
		return fmt.Errorf("tool %q has no handler", t.Name)
	}
	for i, example := range t.Examples {
		if strings.TrimSpace(example.Args) == "" {
			return fmt.Errorf("tool %q example %d has no arguments", t.Name, i)
		}
	}
	return nil
}

func (t Tool[In, Out]) Doc() (Doc, error) {
	input, err := jsonschema.For[In](nil)
	if err != nil {
		return Doc{}, fmt.Errorf("tool %q input schema: %w", t.Name, err)
	}
	output, err := jsonschema.For[Out](nil)
	if err != nil {
		return Doc{}, fmt.Errorf("tool %q output schema: %w", t.Name, err)
	}
	return Doc{
		Kind:         KindTool,
		ID:           t.Name,
		Title:        t.Title,
		Description:  t.description(),
		InputSchema:  input,
		OutputSchema: output,
		Annotations:  t.annotations(),
	}, nil
}

func (t Tool[In, Out]) register(server *mcp.Server) error {
	mcp.AddTool(server, &mcp.Tool{
		Name:        t.Name,
		Title:       t.Title,
		Description: t.description(),
		Annotations: t.annotations(),
	}, t.Handler)
	return nil
}

func (t Tool[In, Out]) annotations() *mcp.ToolAnnotations {
	openWorld := t.OpenWorld
	return &mcp.ToolAnnotations{
		Title:          t.Title,
		ReadOnlyHint:   t.ReadOnly,
		IdempotentHint: t.Idempotent,
		OpenWorldHint:  &openWorld,
	}
}

func (t Tool[In, Out]) description() string {
	return describe(t.Summary, t.Details, t.Examples)
}

// Resource is a read-only thing addressed by URI. A URI containing a {placeholder} is registered
// as a template, so one declaration covers a family of resources.
type Resource struct {
	// URI, e.g. "wowsims://item/{id}". Must be absolute.
	URI string
	// Name is a short identifier, e.g. "item".
	Name string
	// Title is a short human-readable label.
	Title string
	// Summary is one line saying what the resource holds. Required.
	Summary string
	// Details is optional prose.
	Details string
	// Examples are concrete URIs worth showing.
	Examples []Example

	MIMEType string
	Handler  mcp.ResourceHandler
}

func (r Resource) Kind() Kind { return KindResource }
func (r Resource) ID() string { return r.URI }

// IsTemplate reports whether the URI is a template rather than a single address.
func (r Resource) IsTemplate() bool { return strings.Contains(r.URI, "{") }

func (r Resource) Validate() error {
	if !strings.Contains(r.URI, "://") {
		return fmt.Errorf("resource URI %q must be absolute", r.URI)
	}
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("resource %q has no name", r.URI)
	}
	if strings.TrimSpace(r.Summary) == "" {
		return fmt.Errorf("resource %q has no summary", r.URI)
	}
	if r.Handler == nil {
		return fmt.Errorf("resource %q has no handler", r.URI)
	}
	return nil
}

func (r Resource) Doc() (Doc, error) {
	return Doc{
		Kind:        KindResource,
		ID:          r.URI,
		Title:       r.Title,
		Description: describe(r.Summary, r.Details, r.Examples),
		MIMEType:    r.MIMEType,
	}, nil
}

func (r Resource) register(server *mcp.Server) error {
	description := describe(r.Summary, r.Details, r.Examples)
	if r.IsTemplate() {
		server.AddResourceTemplate(&mcp.ResourceTemplate{
			URITemplate: r.URI,
			Name:        r.Name,
			Title:       r.Title,
			Description: description,
			MIMEType:    r.MIMEType,
		}, r.Handler)
		return nil
	}
	server.AddResource(&mcp.Resource{
		URI:         r.URI,
		Name:        r.Name,
		Title:       r.Title,
		Description: description,
		MIMEType:    r.MIMEType,
	}, r.Handler)
	return nil
}

// Prompt is a workflow an agent can be handed. Prompts are where multi-step strategies live --
// a search that runs across several tool calls belongs in a prompt, not in server state.
type Prompt struct {
	Name string
	// Title is a short human-readable label.
	Title string
	// Summary is one line saying what the workflow achieves. Required.
	Summary string
	// Details is optional prose.
	Details string
	// Arguments the workflow takes.
	Arguments []Argument

	Handler mcp.PromptHandler
}

func (p Prompt) Kind() Kind { return KindPrompt }
func (p Prompt) ID() string { return p.Name }

func (p Prompt) Validate() error {
	if !toolNamePattern.MatchString(p.Name) {
		return fmt.Errorf("prompt name %q must be snake_case", p.Name)
	}
	if strings.TrimSpace(p.Summary) == "" {
		return fmt.Errorf("prompt %q has no summary", p.Name)
	}
	if p.Handler == nil {
		return fmt.Errorf("prompt %q has no handler", p.Name)
	}
	for _, argument := range p.Arguments {
		if strings.TrimSpace(argument.Description) == "" {
			return fmt.Errorf("prompt %q argument %q has no description", p.Name, argument.Name)
		}
	}
	return nil
}

func (p Prompt) Doc() (Doc, error) {
	text, err := p.Render(context.Background(), nil)
	if err != nil {
		return Doc{}, fmt.Errorf("prompt %q: %w", p.Name, err)
	}

	return Doc{
		Kind:        KindPrompt,
		ID:          p.Name,
		Title:       p.Title,
		Description: describe(p.Summary, p.Details, nil),
		Arguments:   p.Arguments,
		Text:        text,
	}, nil
}

// Render runs the prompt and returns its text. Handlers fill their own gaps, so rendering with no
// arguments produces the workflow with placeholders where the specifics would go.
func (p Prompt) Render(ctx context.Context, arguments map[string]string) (string, error) {
	if arguments == nil {
		arguments = map[string]string{}
	}

	result, err := p.Handler(ctx, &mcp.GetPromptRequest{Params: &mcp.GetPromptParams{Name: p.Name, Arguments: arguments}})
	if err != nil {
		return "", err
	}

	var text strings.Builder
	for _, message := range result.Messages {
		if content, ok := message.Content.(*mcp.TextContent); ok {
			text.WriteString(content.Text)
		}
	}
	return text.String(), nil
}

func (p Prompt) register(server *mcp.Server) error {
	arguments := make([]*mcp.PromptArgument, 0, len(p.Arguments))
	for _, argument := range p.Arguments {
		arguments = append(arguments, &mcp.PromptArgument{
			Name:        argument.Name,
			Description: argument.Description,
			Required:    argument.Required,
		})
	}
	server.AddPrompt(&mcp.Prompt{
		Name:        p.Name,
		Title:       p.Title,
		Description: describe(p.Summary, p.Details, nil),
		Arguments:   arguments,
	}, p.Handler)
	return nil
}

// describe folds a declaration into the single description string the protocol carries. Agents
// see the summary first, then the detail, then worked examples.
func describe(summary, details string, examples []Example) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(summary))

	if trimmed := strings.TrimSpace(details); trimmed != "" {
		b.WriteString("\n\n")
		b.WriteString(trimmed)
	}

	if len(examples) > 0 {
		b.WriteString("\n\nExamples:")
		for _, example := range examples {
			b.WriteString("\n- ")
			b.WriteString(strings.TrimSpace(example.Description))
			b.WriteString(": ")
			b.WriteString(strings.TrimSpace(example.Args))
		}
	}

	return b.String()
}
