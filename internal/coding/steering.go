package coding

import (
	"context"
	"strings"

	"github.com/yuanbohan/pia/internal/agent"
)

type sessionSteeringSource struct {
	session *Session
	control *activeAdvance
}

func (source sessionSteeringSource) Drain() []string {
	return source.session.drainSteering(source.control, false)
}

func (source sessionSteeringSource) DrainOrSeal() []string {
	return source.session.drainSteering(source.control, true)
}

// Steer admits input for the current Engine invocation to consume at its next
// safe boundary. A nil error acknowledges ownership transfer, not completion.
func (s *Session) Steer(input string) error {
	if err := validateInput(input); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lifetime != sessionOpen {
		return ErrSessionClosed
	}
	if s.active == nil ||
		!s.active.acceptingSteering ||
		context.Cause(s.active.ctx) != nil {
		return ErrSteerUnavailable
	}
	s.active.pendingSteering = append(
		s.active.pendingSteering,
		strings.Clone(input),
	)
	return nil
}

func (s *Session) openSteering(
	control *activeAdvance,
) agent.SteeringSource {
	s.mu.Lock()
	if s.active == control &&
		s.lifetime == sessionOpen &&
		context.Cause(control.ctx) == nil {
		control.acceptingSteering = true
	}
	s.mu.Unlock()
	return sessionSteeringSource{session: s, control: control}
}

func (s *Session) pauseSteering(control *activeAdvance) {
	s.mu.Lock()
	if s.active == control {
		control.acceptingSteering = false
	}
	s.mu.Unlock()
}

func (s *Session) drainSteering(
	control *activeAdvance,
	sealIfEmpty bool,
) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.active != control ||
		s.lifetime != sessionOpen ||
		context.Cause(control.ctx) != nil ||
		!control.acceptingSteering {
		if s.active == control && sealIfEmpty {
			control.acceptingSteering = false
		}
		return nil
	}
	if len(control.pendingSteering) == 0 {
		if sealIfEmpty {
			control.acceptingSteering = false
		}
		return nil
	}

	pending := control.pendingSteering
	control.pendingSteering = nil
	return pending
}

var _ agent.SteeringSource = sessionSteeringSource{}
