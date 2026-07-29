package coding

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/observation"
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
	Excluded       []int
}

type compactionPlan struct {
	FirstKept           int
	MessagesToSummarize []ai.Message
	TurnPrefixMessages  []ai.Message
	SplitTurn           bool
}

type compactionModelSource struct {
	Messages        []ai.Message
	Positions       []int
	PreviousSummary string
	HasSummary      bool
	Excluded        []int
}

func (c *Session) compactBeforeRun(ctx context.Context, userInput string) error {
	if !c.compaction.enabled() {
		return nil
	}

	history, previousProjection := c.compactionSnapshot()
	return c.compactContext(
		ctx,
		history,
		previousProjection,
		nil,
		[]ai.Message{ai.UserMessage{Content: userInput}},
		false,
	)
}

func (c *Session) compactAfterOverflow(ctx context.Context, excludedPosition int) error {
	history, previousProjection := c.compactionSnapshot()
	if excludedPosition < 0 || excludedPosition >= len(history) {
		return fmt.Errorf(
			"overflow terminal position %d is outside History length %d",
			excludedPosition,
			len(history),
		)
	}
	return c.compactContext(
		ctx,
		history,
		previousProjection,
		[]int{excludedPosition},
		nil,
		true,
	)
}

func (c *Session) compactContext(
	ctx context.Context,
	history []ai.Message,
	previousProjection *compactionProjection,
	additionalExclusions []int,
	pendingMessages []ai.Message,
	forced bool,
) (err error) {
	source, err := buildCompactionModelSource(history, previousProjection, additionalExclusions)
	if err != nil {
		return fmt.Errorf("coding: build current context projection: %w", err)
	}
	currentMessages := source.currentMessages()
	projectedRequest := c.modelRequest(currentMessages, pendingMessages)
	if !forced && ai.EstimateRequestTokens(projectedRequest).Tokens < c.compaction.Threshold {
		return nil
	}

	reason := observation.CompactionReasonThreshold
	if forced {
		reason = observation.CompactionReasonOverflow
	}
	c.observer.Observe(observation.Compaction{
		Phase:  observation.PhaseStarted,
		Reason: reason,
	})
	defer func() {
		c.observer.Observe(observation.Compaction{
			Phase:   observation.PhaseSettled,
			Reason:  reason,
			Outcome: observation.OutcomeFromError(err),
		})
	}()

	plan, err := chooseCompactionPlanWithPending(
		source.Messages,
		0,
		pendingMessages,
		c.info.SystemPrompt,
		c.info.Tools,
		c.compaction,
		historyBeforeProjection(history, previousProjection),
	)
	if err != nil {
		return fmt.Errorf("coding: prepare context compaction: %w", err)
	}
	if plan.FirstKept < 0 || plan.FirstKept >= len(source.Positions) {
		return fmt.Errorf(
			"coding: prepare context compaction: first-kept source index %d is outside %d usable messages",
			plan.FirstKept,
			len(source.Positions),
		)
	}
	firstKept := source.Positions[plan.FirstKept]
	// Recovery exclusions are tool-call-free error assistants, so scanning the
	// raw prefix produces the same deterministic file-operation set without
	// cloning every historical message.
	fileOperationHistory := history[:firstKept]

	summary, err := c.executeCompactionPlan(
		ctx,
		plan,
		source.PreviousSummary,
		fileOperationHistory,
	)
	if err != nil {
		return fmt.Errorf("coding: compact context: %w", err)
	}
	projection := compactionProjection{
		Summary:        summary,
		FirstKept:      firstKept,
		UsageValidFrom: len(history),
		Excluded:       exclusionsAtOrAfter(source.Excluded, firstKept),
	}
	candidate, err := projectedMessages(history, &projection)
	if err != nil {
		return fmt.Errorf("coding: build compacted context projection: %w", err)
	}
	if err := validateWorkingContext(candidate); err != nil {
		return fmt.Errorf("coding: validate compacted context: %w", err)
	}
	if len(pendingMessages) == 0 {
		if err := validateContinuationTail(candidate); err != nil {
			return fmt.Errorf("coding: validate compacted continuation context: %w", err)
		}
	}
	candidateRequest := c.modelRequest(candidate, pendingMessages)
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

	// The final cancellation check and metadata publish contain no intervening
	// asynchronous operation. Working Context is derived from this projection
	// at the next execution boundary, so there is no second mutable owner to
	// replace. Cancellation after this commit point does not roll it back.
	c.publishProjection(projection)
	return nil
}

func (c *Session) modelRequest(messages, pendingMessages []ai.Message) ai.Request {
	withPending := make([]ai.Message, 0, len(messages)+len(pendingMessages))
	withPending = append(withPending, messages...)
	withPending = append(withPending, pendingMessages...)
	return ai.Request{
		SystemPrompt: c.info.SystemPrompt,
		Messages:     withPending,
		Tools:        c.info.Tools,
	}
}

func projectedMessages(history []ai.Message, projection *compactionProjection) ([]ai.Message, error) {
	if projection == nil {
		return ai.CloneMessages(history), nil
	}
	if err := validateCompactionProjection(history, projection); err != nil {
		return nil, err
	}
	retained, _ := visibleHistoryRange(
		history,
		projection.FirstKept,
		len(history),
		projection.UsageValidFrom,
		projection.Excluded,
	)
	projected := make([]ai.Message, 0, 1+len(retained))
	projected = append(projected, syntheticSummaryMessage(projection.Summary))
	projected = append(projected, retained...)
	return projected, nil
}

func buildCompactionModelSource(
	history []ai.Message,
	projection *compactionProjection,
	additionalExclusions []int,
) (compactionModelSource, error) {
	source := compactionModelSource{}
	firstKept := 0
	usageValidFrom := 0
	if projection != nil {
		if err := validateCompactionProjection(history, projection); err != nil {
			return compactionModelSource{}, err
		}
		firstKept = projection.FirstKept
		usageValidFrom = projection.UsageValidFrom
		source.PreviousSummary = projection.Summary
		source.HasSummary = true
	}

	exclusions, err := mergeExclusions(history, firstKept, projection, additionalExclusions)
	if err != nil {
		return compactionModelSource{}, err
	}
	source.Messages, source.Positions = visibleHistoryRange(
		history,
		firstKept,
		len(history),
		usageValidFrom,
		exclusions,
	)
	source.Excluded = exclusions
	return source, nil
}

func (source compactionModelSource) currentMessages() []ai.Message {
	projected := make([]ai.Message, 0, len(source.Messages)+1)
	if source.HasSummary {
		projected = append(projected, syntheticSummaryMessage(source.PreviousSummary))
	}
	projected = append(projected, source.Messages...)
	return projected
}

func validateCompactionProjection(history []ai.Message, projection *compactionProjection) error {
	if projection.FirstKept < 0 || projection.FirstKept > len(history) {
		return fmt.Errorf("first-kept index %d is outside History length %d", projection.FirstKept, len(history))
	}
	if projection.UsageValidFrom < projection.FirstKept || projection.UsageValidFrom > len(history) {
		return fmt.Errorf(
			"usage-valid index %d is outside retained History range [%d,%d]",
			projection.UsageValidFrom,
			projection.FirstKept,
			len(history),
		)
	}
	previous := -1
	for _, position := range projection.Excluded {
		if position < projection.FirstKept || position >= len(history) {
			return fmt.Errorf(
				"excluded History position %d is outside retained range [%d,%d)",
				position,
				projection.FirstKept,
				len(history),
			)
		}
		if position <= previous {
			return fmt.Errorf("excluded History positions are not strictly increasing")
		}
		previous = position
	}
	return nil
}

func mergeExclusions(
	history []ai.Message,
	firstKept int,
	projection *compactionProjection,
	additional []int,
) ([]int, error) {
	var exclusions []int
	if projection != nil {
		exclusions = append(exclusions, projection.Excluded...)
	}
	exclusions = append(exclusions, additional...)
	sort.Ints(exclusions)

	merged := exclusions[:0]
	for _, position := range exclusions {
		if position < firstKept || position >= len(history) {
			return nil, fmt.Errorf(
				"excluded History position %d is outside retained range [%d,%d)",
				position,
				firstKept,
				len(history),
			)
		}
		if len(merged) == 0 || merged[len(merged)-1] != position {
			merged = append(merged, position)
		}
	}
	return merged, nil
}

func visibleHistoryRange(
	history []ai.Message,
	start, end, usageValidFrom int,
	exclusions []int,
) ([]ai.Message, []int) {
	excluded := make(map[int]struct{}, len(exclusions))
	for _, position := range exclusions {
		excluded[position] = struct{}{}
	}

	messages := make([]ai.Message, 0, end-start)
	positions := make([]int, 0, end-start)
	for position := start; position < end; position++ {
		if _, skip := excluded[position]; skip {
			continue
		}
		message := ai.CloneMessage(history[position])
		if position < usageValidFrom {
			message = clearMessageUsage(message)
		}
		messages = append(messages, message)
		positions = append(positions, position)
	}
	return messages, positions
}

func clearMessageUsage(message ai.Message) ai.Message {
	assistant, ok := message.(ai.AssistantMessage)
	if !ok {
		return message
	}
	assistant.Usage = ai.Usage{}
	return assistant
}

func exclusionsAtOrAfter(exclusions []int, firstKept int) []int {
	index := sort.SearchInts(exclusions, firstKept)
	return append([]int(nil), exclusions[index:]...)
}

func validateContinuationTail(messages []ai.Message) error {
	if len(messages) == 0 {
		return errors.New("working context is empty")
	}
	switch messages[len(messages)-1].(type) {
	case ai.UserMessage, ai.ToolResultMessage:
		return nil
	default:
		return fmt.Errorf("working context ends with %T instead of user or tool result", messages[len(messages)-1])
	}
}

func chooseCompactionPlan(
	history []ai.Message,
	boundaryStart int,
	userInput string,
	systemPrompt string,
	tools []ai.ToolSchema,
	policy compactionPolicy,
) (compactionPlan, error) {
	var priorFileOperationHistory []ai.Message
	if boundaryStart >= 0 && boundaryStart <= len(history) {
		priorFileOperationHistory = history[:boundaryStart]
	}
	return chooseCompactionPlanWithPending(
		history,
		boundaryStart,
		[]ai.Message{ai.UserMessage{Content: userInput}},
		systemPrompt,
		tools,
		policy,
		priorFileOperationHistory,
	)
}

func chooseCompactionPlanWithPending(
	history []ai.Message,
	boundaryStart int,
	pendingMessages []ai.Message,
	systemPrompt string,
	tools []ai.ToolSchema,
	policy compactionPolicy,
	priorFileOperationHistory []ai.Message,
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
	operations := extractFileOperations(priorFileOperationHistory)
	operations.add(history[boundaryStart:firstKept])
	retainedTokens := estimateMessagesTokens(history[firstKept:])
	fixedTokens := ai.EstimateRequestTokens(ai.Request{
		SystemPrompt: systemPrompt,
		Tools:        tools,
	}).Tokens + estimateMessagesTokens(pendingMessages)
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

func historyBeforeProjection(
	history []ai.Message,
	projection *compactionProjection,
) []ai.Message {
	if projection == nil {
		return nil
	}
	return history[:projection.FirstKept]
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

func (c *Session) executeCompactionPlan(
	ctx context.Context,
	plan compactionPlan,
	previousSummary string,
	fileOperationHistory []ai.Message,
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
	summary += formatFileOperations(extractFileOperations(fileOperationHistory))
	return summary, nil
}

func (c *Session) requestSummary(ctx context.Context, prompt string, requestedOutput int64) (string, error) {
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
