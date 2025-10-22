package payment

import (
	"math/rand"
	"os"
	"tekticket/util"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v82"
)

func TestMain(m *testing.M) {
	if os.Getenv("CI") != "" {
		util.LOGGER.Warn("CI environment, skip integration test")
		return
	}

	InitStripe(os.Getenv("STRIPE_SECRET_KEY"))

	os.Exit(m.Run())
}

func TestPayment(t *testing.T) {
	// Test create payment intent

	minAmount := int64(100)
	maxAmount := int64(99_999_999)

	// Pick a random value in the valid range
	amount := minAmount + rand.Int63n(maxAmount-minAmount+1)

	// Test create payment intent
	intent, err := CreatePaymentIntent(amount, stripe.CurrencyVND)
	require.NoError(t, err)
	require.NotNil(t, intent)

	// Confirm payment
	confirm, err := ConfirmPaymentIntent(intent.ID, "pm_card_visa", "https://example.com/return")
	require.NoError(t, err)
	require.NotNil(t, confirm)
	require.Equal(t, confirm.ID, intent.ID)

	// Create a refund
	refund, err := CreateRefund(intent.ID, Duplicate, amount/5) // Partial refund test
	require.NoError(t, err)
	require.NotNil(t, refund)
	require.Equal(t, intent.ID, refund.PaymentIntent.ID)
	require.Equal(t, amount/5, refund.Amount)
}

func TestConfirmPayment(t *testing.T) {

}
