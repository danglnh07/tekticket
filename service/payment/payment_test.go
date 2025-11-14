package payment

import (
	"math/rand"
	"os"
	"tekticket/util"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v82"
)

var (
	minAmount int64 = 100
	maxAmount int64 = 99_999_999
)

// Main entry point of payment test package
func TestMain(m *testing.M) {
	if os.Getenv("CI") != "" {
		util.LOGGER.Warn("CI environment, skip integration test")
		return
	}

	InitStripe(os.Getenv("STRIPE_SECRET_KEY"))
	os.Exit(m.Run())
}

// Helper method: create a payment method using Stripe test token
func CreatePaymentMethod(t *testing.T) *stripe.PaymentMethod {
	// Use Stripe's test token for Visa card
	// Other available tokens: tok_mastercard, tok_amex, tok_discover, etc.
	paymentMethod, err := CreatePaymentMethodFromToken("tok_visa")
	require.NoError(t, err)
	require.NotNil(t, paymentMethod)
	return paymentMethod
}

// Helper method: create a payment intent
func CreatePayment(t *testing.T, amount int64) *stripe.PaymentIntent {
	key := util.RandomString(6)

	// Test create payment intent
	intent, err := CreatePaymentIntent(amount, stripe.CurrencyVND, key)
	require.NoError(t, err)
	require.NotNil(t, intent)
	util.LOGGER.Info("Transaction created", "amount", amount, "status", intent.Status)

	// Try create the same intent. It should return the previous intent instead of creating a new one
	newIntent, err := CreatePaymentIntent(amount, stripe.CurrencyVND, key)
	require.NoError(t, err)
	require.NotNil(t, newIntent)
	require.Equal(t, intent.ID, newIntent.ID)

	return intent
}

// Helper method: confirm a payment
func ConfirmPayment(t *testing.T, intent *stripe.PaymentIntent, method *stripe.PaymentMethod) *stripe.PaymentIntent {
	// Confirm payment
	confirm, err := ConfirmPaymentIntent(intent.ID, method.ID)
	require.NoError(t, err)
	require.NotNil(t, confirm)
	require.Equal(t, confirm.ID, intent.ID)
	util.LOGGER.Info("Payment confimation", "status", confirm.Status)
	return confirm
}

// Test create payment intent
func TestCreatePaymentIntent(t *testing.T) {
	// Pick a random value in the valid range
	amount := minAmount + rand.Int63n(maxAmount-minAmount+1)
	CreatePayment(t, amount)
}

// Test cancel unpaid payment intent
func TestCancelPaymentIntent(t *testing.T) {
	// Pick a random value in the valid range
	amount := minAmount + rand.Int63n(maxAmount-minAmount+1)
	intent := CreatePayment(t, amount)

	// Cancel intent
	require.NoError(t, CancelPaymentIntent(intent.ID))
}

// Test confirm payment
func TestConfirmPayment(t *testing.T) {
	// Pick a random value in the valid range
	amount := minAmount + rand.Int63n(maxAmount-minAmount+1)
	intent := CreatePayment(t, amount)
	method := CreatePaymentMethod(t)
	ConfirmPayment(t, intent, method)
}

// Test get payment intent
func TestGetPaymentIntent(t *testing.T) {
	// Create a payment intent
	amount := minAmount + rand.Int63n(maxAmount-minAmount+1)
	intent := CreatePayment(t, amount)

	// Get payment intent
	getIntent, err := GetPaymentIntent(intent.ID)
	require.NoError(t, err)
	require.NotNil(t, getIntent)
	require.Equal(t, intent.ID, getIntent.ID)
	require.Equal(t, stripe.PaymentIntentStatusRequiresPaymentMethod, getIntent.Status)
}

// Test partial refund
func TestPartialRefund(t *testing.T) {
	amount := minAmount + rand.Int63n(maxAmount-minAmount+1)
	intent := CreatePayment(t, amount)
	method := CreatePaymentMethod(t)
	ConfirmPayment(t, intent, method)

	// Create a refund
	refund, err := CreateRefund(intent.ID, Duplicate, amount/5) // Partial refund test
	require.NoError(t, err)
	require.NotNil(t, refund)
	require.Equal(t, intent.ID, refund.PaymentIntent.ID)
	require.Equal(t, amount/5, refund.Amount)
	util.LOGGER.Info("Partial refund", "status", refund.Status)
}

// Test full refund
func TestFullRefund(t *testing.T) {
	amount := minAmount + rand.Int63n(maxAmount-minAmount+1)
	intent := CreatePayment(t, amount)
	method := CreatePaymentMethod(t)
	ConfirmPayment(t, intent, method)

	// Create a refund
	refund, err := CreateRefund(intent.ID, Duplicate, amount)
	require.NoError(t, err)
	require.NotNil(t, refund)
	require.Equal(t, intent.ID, refund.PaymentIntent.ID)
	require.Equal(t, amount, refund.Amount)
	util.LOGGER.Info("Full refund", "status", refund.Status)
}
