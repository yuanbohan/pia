package coding

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/yuanbohan/pia/internal/ai"
)

type compactionPolicy struct {
	Threshold                int64
	SoftCeiling              int64
	RetainedRawTarget        int64
	SummaryMaxOutput         int64
	SplitTurnPrefixMaxOutput int64
}

func (policy compactionPolicy) enabled() bool {
	return policy != (compactionPolicy{})
}

func (policy compactionPolicy) validate() error {
	if !policy.enabled() {
		return nil
	}
	if policy.Threshold <= 0 {
		return errors.New("threshold must be positive")
	}
	if policy.SoftCeiling <= 0 || policy.SoftCeiling >= policy.Threshold {
		return errors.New("soft ceiling must be positive and below threshold")
	}
	if policy.RetainedRawTarget <= 0 {
		return errors.New("retained raw target must be positive")
	}
	if policy.SummaryMaxOutput <= 0 {
		return errors.New("summary max output must be positive")
	}
	if policy.SplitTurnPrefixMaxOutput <= 0 {
		return errors.New("split-turn prefix max output must be positive")
	}
	return nil
}

type compactionProjection struct {
	Summary        string
	FirstKept      int
	UsageValidFrom int
}

type compactionPlan struct {
	FirstKept           int
	MessagesToSummarize []ai.Message
	TurnPrefixMessages  []ai.Message
	SplitTurn           bool
}

func (c *conversation) compactBeforeRun(ctx context.Context, userInput string) error {
	if !c.compaction.enabled() {
		return nil
	}

	history, previousProjection := c.compactionSnapshot()
	currentMessages, err := projectedMessages(history, previousProjection)
	if err != nil {
		return fmt.Errorf("coding: build current context projection: %w", err)
	}
	projectedRequest := c.codingRequest(currentMessages, userInput)
	if ai.EstimateRequestTokens(projectedRequest).Tokens < c.compaction.Threshold {
		return nil
	}

	boundaryStart := 0
	previousSummary := ""
	if previousProjection != nil {
		boundaryStart = previousProjection.FirstKept
		previousSummary = previousProjection.Summary
	}
	plan, err := chooseCompactionPlan(
		history,
		boundaryStart,
		userInput,
		c.systemPrompt,
		c.tools,
		c.compaction,
	)
	if err != nil {
		return fmt.Errorf("coding: prepare context compaction: %w", err)
	}

	summary, err := c.executeCompactionPlan(ctx, history, plan, previousSummary)
	if err != nil {
		return fmt.Errorf("coding: compact context: %w", err)
	}
	candidate := compactedMessages(summary, history, plan.FirstKept)
	if err := validateWorkingContext(candidate); err != nil {
		return fmt.Errorf("coding: validate compacted context: %w", err)
	}
	candidateRequest := c.codingRequest(candidate, userInput)
	candidateTokens := ai.EstimateRequestTokens(candidateRequest).Tokens
	if candidateTokens >= c.compaction.Threshold {
		return fmt.Errorf(
			"coding: compacted context is still at or above threshold: %d >= %d tokens",
			candidateTokens,
			c.compaction.Threshold,
		)
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}

	// The final cancellation check, idle-only replacement, and metadata publish
	// contain no intervening asynchronous operation. Cancellation after this
	// commit point does not roll back the valid projection.
	if err := c.core.ReplaceWorkingContext(candidate); err != nil {
		return fmt.Errorf("coding: replace compacted Working Context: %w", err)
	}
	c.publishProjection(compactionProjection{
		Summary:        summary,
		FirstKept:      plan.FirstKept,
		UsageValidFrom: len(history),
	})
	return nil
}

func (c *conversation) codingRequest(messages []ai.Message, userInput string) ai.Request {
	withInput := make([]ai.Message, 0, len(messages)+1)
	withInput = append(withInput, messages...)
	withInput = append(withInput, ai.UserMessage{Content: userInput})
	return ai.Request{
		SystemPrompt: c.systemPrompt,
		Messages:     withInput,
		Tools:        c.tools,
	}
}

func projectedMessages(history []ai.Message, projection *compactionProjection) ([]ai.Message, error) {
	if projection == nil {
		return history, nil
	}
	if projection.FirstKept < 0 || projection.FirstKept > len(history) {
		return nil, fmt.Errorf("first-kept index %d is outside History length %d", projection.FirstKept, len(history))
	}
	if projection.UsageValidFrom < projection.FirstKept || projection.UsageValidFrom > len(history) {
		return nil, fmt.Errorf(
			"usage-valid index %d is outside retained History range [%d,%d]",
			projection.UsageValidFrom,
			projection.FirstKept,
			len(history),
		)
	}

	projected := make([]ai.Message, 0, 1+len(history)-projection.FirstKept)
	projected = append(projected, syntheticSummaryMessage(projection.Summary))
	retained := ai.CloneMessages(history[projection.FirstKept:])
	clearAssistantUsage(retained[:projection.UsageValidFrom-projection.FirstKept])
	projected = append(projected, retained...)
	return projected, nil
}

func chooseCompactionPlan(
	history []ai.Message,
	boundaryStart int,
	userInput string,
	systemPrompt string,
	tools []ai.ToolSchema,
	policy compactionPolicy,
) (compactionPlan, error) {
	if boundaryStart < 0 || boundaryStart >= len(history) {
		return compactionPlan{}, fmt.Errorf("no retained History is available to compact")
	}
	cutPoints := validCutPoints(history, boundaryStart)
	if len(cutPoints) == 0 {
		return compactionPlan{}, fmt.Errorf("no protocol-safe History cut point is available")
	}
	if len(cutPoints) == 1 && cutPoints[0] == boundaryStart {
		return compactionPlan{}, fmt.Errorf("no protocol-safe History prefix can be summarized")
	}

	firstKept := targetCutPoint(history, boundaryStart, cutPoints, policy.RetainedRawTarget)
	operations := extractFileOperations(history[:firstKept])
	retainedTokens := estimateMessagesTokens(history[firstKept:])
	fixedTokens := ai.EstimateRequestTokens(ai.Request{
		SystemPrompt: systemPrompt,
		Tools:        tools,
	}).Tokens + ai.EstimateMessageTokens(ai.UserMessage{Content: userInput})
	advance := func(next int) {
		operations.add(history[firstKept:next])
		retainedTokens -= estimateMessagesTokens(history[firstKept:next])
		firstKept = next
	}
	for {
		plan := planForCut(history, boundaryStart, firstKept)
		if len(plan.MessagesToSummarize) == 0 && len(plan.TurnPrefixMessages) == 0 {
			next, ok := nextCutPoint(cutPoints, firstKept)
			if !ok {
				return compactionPlan{}, fmt.Errorf("no protocol-safe History prefix can be summarized")
			}
			advance(next)
			continue
		}

		forecast := forecastCandidateTokens(
			plan,
			retainedTokens,
			fixedTokens,
			formatFileOperations(operations),
			policy,
		)
		if forecast <= policy.SoftCeiling {
			return plan, nil
		}
		next, ok := nextCutPoint(cutPoints, firstKept)
		if !ok {
			return plan, nil
		}
		advance(next)
	}
}

func validCutPoints(history []ai.Message, start int) []int {
	points := make([]int, 0, len(history)-start)
	for index := start; index < len(history); index++ {
		switch message := history[index].(type) {
		case ai.UserMessage, ai.AssistantMessage:
			points = append(points, index)
		case *ai.UserMessage:
			if message != nil {
				points = append(points, index)
			}
		case *ai.AssistantMessage:
			if message != nil {
				points = append(points, index)
			}
		}
	}
	return points
}

func targetCutPoint(history []ai.Message, start int, cutPoints []int, retainedTarget int64) int {
	firstKept := cutPoints[0]
	var accumulated int64
	for index := len(history) - 1; index >= start; index-- {
		accumulated += ai.EstimateMessageTokens(history[index])
		if accumulated < retainedTarget {
			continue
		}
		for _, point := range cutPoints {
			if point >= index {
				return point
			}
		}
		break
	}
	return firstKept
}

func nextCutPoint(cutPoints []int, current int) (int, bool) {
	index := sort.Search(len(cutPoints), func(index int) bool {
		return cutPoints[index] > current
	})
	if index == len(cutPoints) {
		return 0, false
	}
	return cutPoints[index], true
}

func planForCut(history []ai.Message, boundaryStart, firstKept int) compactionPlan {
	plan := compactionPlan{FirstKept: firstKept}
	if isUserMessage(history[firstKept]) {
		plan.MessagesToSummarize = ai.CloneMessages(history[boundaryStart:firstKept])
		return plan
	}

	turnStart := findTurnStart(history, firstKept, boundaryStart)
	if turnStart >= boundaryStart {
		plan.SplitTurn = true
		plan.MessagesToSummarize = ai.CloneMessages(history[boundaryStart:turnStart])
		plan.TurnPrefixMessages = ai.CloneMessages(history[turnStart:firstKept])
		return plan
	}
	plan.MessagesToSummarize = ai.CloneMessages(history[boundaryStart:firstKept])
	return plan
}

func findTurnStart(history []ai.Message, entry, start int) int {
	for index := entry; index >= start; index-- {
		if isUserMessage(history[index]) {
			return index
		}
	}
	return -1
}

func isUserMessage(message ai.Message) bool {
	switch message := message.(type) {
	case ai.UserMessage:
		return true
	case *ai.UserMessage:
		return message != nil
	default:
		return false
	}
}

func forecastCandidateTokens(
	plan compactionPlan,
	retainedTokens int64,
	fixedTokens int64,
	formattedFileOperations string,
	policy compactionPolicy,
) int64 {
	summaryBudget := policy.SummaryMaxOutput
	if plan.SplitTurn {
		summaryBudget += policy.SplitTurnPrefixMaxOutput
	}
	envelopeTokens := ai.EstimateMessageTokens(syntheticSummaryMessage(formattedFileOperations))
	// The placeholder already accounts for the synthetic summary envelope and
	// deterministic file lists. Add the maximum model-generated summary size
	// without allocating a large fake string for every candidate cut point.
	return fixedTokens + retainedTokens + envelopeTokens + summaryBudget
}

func estimateMessagesTokens(messages []ai.Message) int64 {
	var tokens int64
	for _, message := range messages {
		tokens += ai.EstimateMessageTokens(message)
	}
	return tokens
}

func (c *conversation) executeCompactionPlan(
	ctx context.Context,
	history []ai.Message,
	plan compactionPlan,
	previousSummary string,
) (string, error) {
	var historySummary string
	if len(plan.MessagesToSummarize) > 0 {
		prompt := buildSummarizationPrompt(plan.MessagesToSummarize, previousSummary)
		generated, err := c.requestSummary(ctx, prompt, c.compaction.SummaryMaxOutput)
		if err != nil {
			return "", err
		}
		historySummary = generated
	} else if previousSummary != "" {
		historySummary = previousSummary
	} else {
		historySummary = "No prior history."
	}

	summary := historySummary
	if plan.SplitTurn {
		prefixPrompt := buildTurnPrefixPrompt(plan.TurnPrefixMessages)
		prefixSummary, err := c.requestSummary(ctx, prefixPrompt, c.compaction.SplitTurnPrefixMaxOutput)
		if err != nil {
			return "", err
		}
		summary = historySummary + "\n\n---\n\n**Turn Context (split turn):**\n\n" + prefixSummary
	}
	summary += formatFileOperations(extractFileOperations(history[:plan.FirstKept]))
	return summary, nil
}

func (c *conversation) requestSummary(ctx context.Context, prompt string, requestedOutput int64) (string, error) {
	request := ai.Request{
		SystemPrompt: summarizationSystemPrompt,
		Messages: []ai.Message{
			ai.UserMessage{Content: prompt},
		},
	}
	projectedInput := ai.EstimateRequestTokens(request).Tokens
	request.MaxOutputTokens = c.requestLimits.ClampOutputTokens(projectedInput, requestedOutput)
	message, err := receiveSummary(ctx, c.provider.Stream(ctx, request))
	if err != nil {
		return "", err
	}
	return summaryText(message)
}

func receiveSummary(ctx context.Context, stream ai.Stream) (ai.AssistantMessage, error) {
	if stream == nil {
		return ai.AssistantMessage{}, errors.New("summary Provider returned a nil stream")
	}
	for {
		event, receiveErr := stream.Receive()
		switch event := event.(type) {
		case ai.DoneEvent:
			return event.Message, nil
		case *ai.DoneEvent:
			if event != nil {
				return event.Message, nil
			}
			return ai.AssistantMessage{}, errors.New("summary Provider returned a nil done event")
		case ai.ErrorEvent:
			return ai.AssistantMessage{}, summaryTerminalError(ctx, event.Message)
		case *ai.ErrorEvent:
			if event != nil {
				return ai.AssistantMessage{}, summaryTerminalError(ctx, event.Message)
			}
			return ai.AssistantMessage{}, errors.New("summary Provider returned a nil error event")
		}
		if receiveErr != nil {
			if cause := context.Cause(ctx); cause != nil {
				return ai.AssistantMessage{}, cause
			}
			if errors.Is(receiveErr, io.EOF) {
				return ai.AssistantMessage{}, errors.New("summary Provider stream ended before terminal event")
			}
			return ai.AssistantMessage{}, fmt.Errorf("receive summary Provider stream: %w", receiveErr)
		}
		if event == nil {
			return ai.AssistantMessage{}, errors.New("summary Provider returned a nil event without an error")
		}
	}
}

func summaryTerminalError(ctx context.Context, message ai.AssistantMessage) error {
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	if strings.TrimSpace(message.ErrorMessage) == "" {
		return fmt.Errorf("summary Provider failed with stop reason %q", message.StopReason)
	}
	return fmt.Errorf("summary Provider failed: %s", message.ErrorMessage)
}

func summaryText(message ai.AssistantMessage) (string, error) {
	var text []string
	for _, content := range message.Content {
		switch content := content.(type) {
		case ai.TextContent:
			text = append(text, content.Text)
		case *ai.TextContent:
			if content != nil {
				text = append(text, content.Text)
			}
		case ai.ThinkingContent, *ai.ThinkingContent:
			// Reasoning is not part of the summary checkpoint.
		case ai.ToolCall, *ai.ToolCall:
			return "", errors.New("summary Provider returned an unexpected tool call")
		}
	}
	if message.StopReason != ai.StopReasonStop && message.StopReason != ai.StopReasonLength {
		return "", fmt.Errorf("summary Provider returned unexpected stop reason %q", message.StopReason)
	}
	joined := strings.Join(text, "\n")
	if strings.TrimSpace(joined) == "" {
		return "", errors.New("summary Provider returned empty text")
	}
	return joined, nil
}

func compactedMessages(summary string, history []ai.Message, firstKept int) []ai.Message {
	retained := ai.CloneMessages(history[firstKept:])
	clearAssistantUsage(retained)
	messages := make([]ai.Message, 0, 1+len(retained))
	messages = append(messages, syntheticSummaryMessage(summary))
	messages = append(messages, retained...)
	return messages
}

func clearAssistantUsage(messages []ai.Message) {
	for index, message := range messages {
		switch message := message.(type) {
		case ai.AssistantMessage:
			message.Usage = ai.Usage{}
			messages[index] = message
		case *ai.AssistantMessage:
			if message != nil {
				copy := ai.CloneAssistantMessage(*message)
				copy.Usage = ai.Usage{}
				messages[index] = copy
			}
		}
	}
}

func validateWorkingContext(messages []ai.Message) error {
	pending := make(map[string]string)
	for index, message := range messages {
		switch message := message.(type) {
		case ai.UserMessage:
			if len(pending) > 0 {
				return fmt.Errorf("user message %d appears before pending tool results", index)
			}
		case ai.AssistantMessage:
			if len(pending) > 0 {
				return fmt.Errorf("assistant message %d appears before pending tool results", index)
			}
			for _, content := range message.Content {
				call, ok := compactionToolCall(content)
				if !ok {
					continue
				}
				if call.ID == "" {
					return fmt.Errorf("assistant message %d has an empty tool-call ID", index)
				}
				if _, duplicate := pending[call.ID]; duplicate {
					return fmt.Errorf("assistant message %d repeats tool-call ID %q", index, call.ID)
				}
				pending[call.ID] = call.Name
			}
		case ai.ToolResultMessage:
			name, ok := pending[message.ToolCallID]
			if !ok {
				return fmt.Errorf("tool result %d has no retained call %q", index, message.ToolCallID)
			}
			if name != message.ToolName {
				return fmt.Errorf("tool result %d name %q does not match retained call %q", index, message.ToolName, name)
			}
			delete(pending, message.ToolCallID)
		case *ai.UserMessage, *ai.AssistantMessage, *ai.ToolResultMessage:
			return fmt.Errorf("working context message %d retained an unexpected pointer form %T", index, message)
		case nil:
			return fmt.Errorf("working context message %d is nil", index)
		}
	}
	if len(pending) > 0 {
		return errors.New("working context ends before all retained tool calls have results")
	}
	return nil
}

func compactionToolCall(content ai.AssistantContent) (ai.ToolCall, bool) {
	switch content := content.(type) {
	case ai.ToolCall:
		return content, true
	case *ai.ToolCall:
		if content != nil {
			return *content, true
		}
	}
	return ai.ToolCall{}, false
}
