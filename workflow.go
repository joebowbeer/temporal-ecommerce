package app

import (
	"github.com/mitchellh/mapstructure"
	"go.temporal.io/sdk/workflow"
	"time"
)

type (
	CartItem struct {
		ProductId int
		Quantity  int
	}

	CartState struct {
		Items []CartItem
		Email string
	}

	UpdateCartMessage struct {
		Remove bool
		Item   CartItem
	}
)

var (
	// Short timeout to consider shopping cart abandoned for development purposes.
	abandonedCartTimeout = 10 * time.Second
)

func CartWorkflow(ctx workflow.Context, state CartState) error {
	// https://docs.temporal.io/docs/concepts/workflows/#workflows-have-options
	logger := workflow.GetLogger(ctx)

	err := workflow.SetQueryHandler(ctx, "getCart", func(input []byte) (CartState, error) {
		return state, nil
	})
	if err != nil {
		logger.Info("SetQueryHandler failed.", "Error", err)
		return err
	}

	channel := workflow.GetSignalChannel(ctx, SignalChannelName)
	checkedOut := false
	sentAbandonedCartEmail := false

	var a *Activities

	for {
		selector := workflow.NewSelector(ctx)
		selector.AddReceive(channel, func(c workflow.ReceiveChannel, _ bool) {
			var signal interface{}
			c.Receive(ctx, &signal)

			var routeSignal RouteSignal
			err := mapstructure.Decode(signal, &routeSignal)
			if err != nil {
				logger.Error("Invalid signal type %v", err)
				return
			}

			switch {
			case routeSignal.Route == RouteTypes.ADD_TO_CART:
				var message AddToCartSignal
				err := mapstructure.Decode(signal, &message)
				if err != nil {
					logger.Error("Invalid signal type %v", err)
					return
				}

				state.AddToCart(message.Item)
			case routeSignal.Route == RouteTypes.REMOVE_FROM_CART:
				var message RemoveFromCartSignal
				err := mapstructure.Decode(signal, &message)
				if err != nil {
					logger.Error("Invalid signal type %v", err)
					return
				}

				state.RemoveFromCart(message.Item)
			case routeSignal.Route == RouteTypes.UPDATE_EMAIL:
				var message UpdateEmailSignal
				err := mapstructure.Decode(signal, &message)
				if err != nil {
					logger.Error("Invalid signal type %v", err)
					return
				}

				state.Email = message.Email
			case routeSignal.Route == RouteTypes.CHECKOUT:
				var message CheckoutSignal
				err := mapstructure.Decode(signal, &message)
				if err != nil {
					logger.Error("Invalid signal type %v", err)
					return
				}

				state.Email = message.Email

				ao := workflow.ActivityOptions{
					StartToCloseTimeout: time.Minute,
				}

				ctx = workflow.WithActivityOptions(ctx, ao)

				err = workflow.ExecuteActivity(ctx, a.CreateStripeCharge, state).Get(ctx, nil)
				if err != nil {
					logger.Error("Error creating stripe charge: %v", err)
					return
				}

				checkedOut = true
			}
		})

		if !sentAbandonedCartEmail && len(state.Items) > 0 {
			selector.AddFuture(workflow.NewTimer(ctx, abandonedCartTimeout), func(f workflow.Future) {
				sentAbandonedCartEmail = true
				ao := workflow.ActivityOptions{
					StartToCloseTimeout: time.Minute,
				}

				ctx = workflow.WithActivityOptions(ctx, ao)

				err := workflow.ExecuteActivity(ctx, a.SendAbandonedCartEmail, state.Email).Get(ctx, nil)
				if err != nil {
					logger.Error("Error sending email %v", err)
					return
				}
			})
		}

		selector.Select(ctx)

		if checkedOut {
			break
		}
	}

	return nil
}

// @@@SNIPSTART temporal-ecommerce-add-and-remove
func (state *CartState) AddToCart(item CartItem) {
	for i := range state.Items {
		if state.Items[i].ProductId != item.ProductId {
			continue
		}

		state.Items[i].Quantity += item.Quantity
		return
	}

	state.Items = append(state.Items, item)
}

func (state *CartState) RemoveFromCart(item CartItem) {
	for i := range state.Items {
		if state.Items[i].ProductId != item.ProductId {
			continue
		}

		state.Items[i].Quantity -= item.Quantity
		if state.Items[i].Quantity <= 0 {
			state.Items = append(state.Items[:i], state.Items[i+1:]...)
		}
		break
	}
}
// @@@SNIPEND
