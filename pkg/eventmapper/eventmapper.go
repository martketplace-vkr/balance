package eventmapper

import (
	"context"

	"github.com/martketplace-vkr/pkg/inbox/dto"
)

type service interface {
	HandleUserSignUp(context.Context, dto.Event) error
}

func GetEventMapper(svc service) map[string]func(context.Context, dto.Event) error {
	return map[string]func(context.Context, dto.Event) error{
		"user_sign_up": svc.HandleUserSignUp,
	}
}
