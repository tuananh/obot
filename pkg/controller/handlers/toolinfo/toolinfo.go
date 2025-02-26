package toolinfo

import (
	"context"
	"errors"
	"fmt"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/controller/creds"
	"github.com/obot-platform/obot/pkg/render"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/sets"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	gptscript *gptscript.GPTScript
}

func New(gptscript *gptscript.GPTScript) *Handler {
	return &Handler{
		gptscript: gptscript,
	}
}

// SetToolInfoStatus will set the tool information for the object. This includes credential information,
// and whether those credentials exist.
// This handler should be used with the generationed.UpdateObservedGeneration to ensure that the processing
// is correctly reported to through the API.
func (h *Handler) SetToolInfoStatus(req router.Request, resp router.Response) (err error) {
	defer func() {
		if err != nil {
			resp.Attributes()["generation:errored"] = true
		}
	}()

	// Get all the credentials that exist in the expected context.
	creds, err := h.gptscript.ListCredentials(req.Ctx, gptscript.ListCredentialsOptions{
		CredentialContexts: []string{req.Name, req.Namespace},
	})
	if err != nil {
		return err
	}

	credsSet := make(sets.Set[string], len(creds))
	for _, cred := range creds {
		credsSet.Insert(cred.ToolName)
	}

	obj := req.Object.(v1.ToolUser)
	tools := obj.GetTools()
	toolInfos := make(map[string]types.ToolInfo, len(tools))

	var (
		toolRef   v1.ToolReference
		credNames []string
	)
	for _, tool := range tools {
		if render.IsExternalTool(tool) {
			credNames, err = h.credentialNamesForNonToolReferences(req.Ctx, tool)
			if err != nil {
				return err
			}
		} else if err = req.Get(&toolRef, req.Namespace, tool); apierror.IsNotFound(err) {
			continue
		} else if err != nil {
			return err
		} else if toolRef.Status.Tool == nil {
			return fmt.Errorf("cannot determine credential status for tool %s: no tool status found", tool)
		} else if err == nil {
			credNames = toolRef.Status.Tool.CredentialNames
			// Clear the field we care about in this loop.
			// This allows us to use the same variable for the whole loop
			// while ensuring that the value we care about is loaded correctly.
			toolRef.Status.Tool.CredentialNames = nil
		}

		toolInfos[tool] = types.ToolInfo{
			CredentialNames: credNames,
			Authorized:      credsSet.HasAll(credNames...),
		}
	}

	obj.SetToolInfos(toolInfos)

	return nil
}

func (h *Handler) RemoveUnneededCredentials(req router.Request, _ router.Response) error {
	creds, err := h.gptscript.ListCredentials(req.Ctx, gptscript.ListCredentialsOptions{
		CredentialContexts: []string{req.Object.GetName()},
	})
	if err != nil || len(creds) == 0 {
		return err
	}

	toolInfos := req.Object.(v1.ToolUser).GetToolInfos()
	credentialNames := make(map[string]struct{}, len(toolInfos))

	for _, cred := range toolInfos {
		for _, name := range cred.CredentialNames {
			credentialNames[name] = struct{}{}
		}
	}

	var knowledgeSets v1.KnowledgeSetList
	switch req.Object.(type) {
	case *v1.Workflow:
		if err := req.List(&knowledgeSets, &kclient.ListOptions{
			Namespace: req.Namespace,
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.workflowName": req.Object.GetName(),
			}),
		}); err != nil {
			return err
		}

	case *v1.Agent:
		if err := req.List(&knowledgeSets, &kclient.ListOptions{
			Namespace: req.Namespace,
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.agentName": req.Object.GetName(),
			}),
		}); err != nil {
			return err
		}
	}

	for _, knowledgeSet := range knowledgeSets.Items {
		var knowledgeSources v1.KnowledgeSourceList
		if err := req.List(&knowledgeSources, &kclient.ListOptions{
			Namespace: req.Namespace,
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.knowledgeSetName": knowledgeSet.Name,
			}),
		}); err != nil {
			return err
		}

		for _, knowledgeSource := range knowledgeSources.Items {
			if sourceType := string(knowledgeSource.Spec.Manifest.GetType()); sourceType != "" {
				credentialNames[sourceType+".sync-file"] = struct{}{}
			}
		}
	}

	for _, cred := range creds {
		if _, ok := credentialNames[cred.ToolName]; !ok {
			if err := h.gptscript.DeleteCredential(req.Ctx, req.Object.GetName(), cred.ToolName); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				return err
			}
		}
	}

	return nil
}

func (h *Handler) credentialNamesForNonToolReferences(ctx context.Context, name string) ([]string, error) {
	prg, err := h.gptscript.LoadFile(ctx, name)
	if err != nil {
		return nil, err
	}

	_, credNames, err := creds.DetermineCredsAndCredNames(prg, prg.ToolSet[prg.EntryToolID], name)
	return credNames, err
}
