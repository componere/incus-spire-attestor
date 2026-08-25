package server

import (
	"context"
	"fmt"
	"slices"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

// resolveInstance locates the unique allowed VM described by claims.
func (s *Service) resolveInstance(ctx context.Context, claims attest.Claims) (attest.Instance, error) {
	lookupCtx, cancel := context.WithTimeout(ctx, s.incusTimeout)
	defer cancel()

	if claims.Project != "" {
		return s.resolveHinted(lookupCtx, claims)
	}
	return s.resolveUnhinted(lookupCtx, claims)
}

// resolveHinted performs one allowlisted project-qualified lookup.
func (s *Service) resolveHinted(ctx context.Context, claims attest.Claims) (attest.Instance, error) {
	if !slices.Contains(s.projects, claims.Project) {
		return attest.Instance{}, fmt.Errorf("%w: project is not allowed", attest.ErrDenied)
	}

	instance, found, err := s.client.Lookup(ctx, claims.Project, claims.Name)
	if err != nil {
		return attest.Instance{}, fmt.Errorf("lookup instance: %w", err)
	}
	if !found {
		return attest.Instance{}, fmt.Errorf("%w: instance not found", attest.ErrDenied)
	}
	if err := attest.MatchClaims(claims, instance); err != nil {
		return attest.Instance{}, err
	}
	return instance, nil
}

// resolveUnhinted searches every allowed project and requires one match.
func (s *Service) resolveUnhinted(ctx context.Context, claims attest.Claims) (attest.Instance, error) {
	var matches []attest.Instance
	for _, project := range s.projects {
		instance, found, err := s.client.Lookup(ctx, project, claims.Name)
		if err != nil {
			return attest.Instance{}, fmt.Errorf("lookup instance: %w", err)
		}
		if !found {
			continue
		}
		if err := attest.MatchClaims(claims, instance); err != nil {
			continue
		}
		matches = append(matches, instance)
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return attest.Instance{}, fmt.Errorf("%w: instance not found", attest.ErrDenied)
	default:
		return attest.Instance{}, fmt.Errorf("%w: multiple matching instances", attest.ErrDenied)
	}
}
