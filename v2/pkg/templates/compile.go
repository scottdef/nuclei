package templates

import (
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/projectdiscovery/nuclei/v2/pkg/protocols"
	"github.com/projectdiscovery/nuclei/v2/pkg/protocols/common/executer"
	"github.com/projectdiscovery/nuclei/v2/pkg/types"
	"github.com/projectdiscovery/nuclei/v2/pkg/workflows"
	"gopkg.in/yaml.v2"
)

// Parse parses a yaml request template file
func Parse(filePath string, options *protocols.ExecuterOptions) (*Template, error) {
	template := &Template{}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	err = yaml.NewDecoder(f).Decode(template)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, ok := template.Info["name"]; !ok {
		return nil, errors.New("no template name field provided")
	}
	if _, ok := template.Info["author"]; !ok {
		return nil, errors.New("no template author field provided")
	}
	if _, ok := template.Info["severity"]; !ok {
		return nil, errors.New("no template severity field provided")
	}
	if templateTags, ok := template.Info["tags"]; ok && len(options.Options.Tags) > 0 {
		if err := matchTemplateWithTags(templateTags.([]interface{}), options.Options); err != nil {
			return nil, err
		}
	}

	// Setting up variables regarding template metadata
	options.TemplateID = template.ID
	options.TemplateInfo = template.Info
	options.TemplatePath = filePath

	// If no requests, and it is also not a workflow, return error.
	if len(template.RequestsDNS)+len(template.RequestsHTTP)+len(template.RequestsFile)+len(template.RequestsNetwork)+len(template.Workflows) == 0 {
		return nil, fmt.Errorf("no requests defined for %s", template.ID)
	}

	// Compile the workflow request
	if len(template.Workflows) > 0 {
		compiled := &template.Workflow
		if err := template.compileWorkflow(options, compiled); err != nil {
			return nil, errors.Wrap(err, "could not compile workflow")
		}
		template.CompiledWorkflow = compiled
	}

	// Compile the requests found
	requests := []protocols.Request{}
	if len(template.RequestsDNS) > 0 {
		for _, req := range template.RequestsDNS {
			requests = append(requests, req)
		}
		template.Executer = executer.NewExecuter(requests, options)
	}
	if len(template.RequestsHTTP) > 0 {
		for _, req := range template.RequestsHTTP {
			requests = append(requests, req)
		}
		template.Executer = executer.NewExecuter(requests, options)
	}
	if len(template.RequestsFile) > 0 {
		for _, req := range template.RequestsFile {
			requests = append(requests, req)
		}
		template.Executer = executer.NewExecuter(requests, options)
	}
	if len(template.RequestsNetwork) > 0 {
		for _, req := range template.RequestsNetwork {
			requests = append(requests, req)
		}
		template.Executer = executer.NewExecuter(requests, options)
	}
	if template.Executer != nil {
		err := template.Executer.Compile()
		if err != nil {
			return nil, errors.Wrap(err, "could not compile request")
		}
		template.TotalRequests += template.Executer.Requests()
	}
	return template, nil
}

// compileWorkflow compiles the workflow for execution
func (t *Template) compileWorkflow(options *protocols.ExecuterOptions, workflows *workflows.Workflow) error {
	for _, workflow := range workflows.Workflows {
		if err := t.parseWorkflow(workflow, options); err != nil {
			return err
		}
	}
	return nil
}

// parseWorkflow parses and compiles all templates in a workflow recursively
func (t *Template) parseWorkflow(workflow *workflows.WorkflowTemplate, options *protocols.ExecuterOptions) error {
	if err := t.parseWorkflowTemplate(workflow, options); err != nil {
		return err
	}
	for _, subtemplates := range workflow.Subtemplates {
		if err := t.parseWorkflow(subtemplates, options); err != nil {
			return err
		}
	}
	for _, matcher := range workflow.Matchers {
		for _, subtemplates := range matcher.Subtemplates {
			if err := t.parseWorkflow(subtemplates, options); err != nil {
				return err
			}
		}
	}
	return nil
}

// parseWorkflowTemplate parses a workflow template creating an executer
func (t *Template) parseWorkflowTemplate(workflow *workflows.WorkflowTemplate, options *protocols.ExecuterOptions) error {
	paths, err := options.Catalogue.GetTemplatePath(workflow.Template)
	if err != nil {
		return errors.Wrap(err, "could not get workflow template")
	}
	for _, path := range paths {
		opts := &protocols.ExecuterOptions{
			Output:      options.Output,
			Options:     options.Options,
			Progress:    options.Progress,
			Catalogue:   options.Catalogue,
			RateLimiter: options.RateLimiter,
			ProjectFile: options.ProjectFile,
		}
		template, err := Parse(path, opts)
		if err != nil {
			return errors.Wrap(err, "could not parse workflow template")
		}
		if template.Executer == nil {
			return errors.New("no executer found for template")
		}
		workflow.Executers = append(workflow.Executers, &workflows.ProtocolExecuterPair{
			Executer: template.Executer,
			Options:  options,
		})
	}
	return nil
}

// matchTemplateWithTags matches if the template matches a tag
func matchTemplateWithTags(tags []interface{}, options *types.Options) error {
	matched := false
mainLoop:
	for _, tag := range options.Tags {
		key, value := getKeyValue(tag)

		for _, templTag := range tags {
			tKey, tValue := getKeyValue(templTag.(string))
			if strings.EqualFold(key, tKey) && strings.EqualFold(value, tValue) {
				matched = true
				break mainLoop
			}
		}
	}
	if !matched {
		return errors.New("could not match template tags with input")
	}
	return nil
}

// getKeyValue returns key value pair for a data string
func getKeyValue(data string) (string, string) {

	var key, value string
	if strings.Contains(data, ":") {
		parts := strings.SplitN(data, ":", 1)
		if len(parts) > 2 {
			key, value = parts[0], parts[1]
		}
	}
	if value == "" {
		value = data
	}
	return key, value
}
