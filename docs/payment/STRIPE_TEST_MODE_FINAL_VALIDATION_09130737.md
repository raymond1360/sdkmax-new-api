# SDKMAX Stripe Test Mode Final Validation 09130737

## Scope

- Project: `D:\sdkmax\new-api`
- Branch: `feature/stripe-one-time-topup-audit-fixes`
- Baseline HEAD before this local submission: `8b0010f9fd81787d2bfa656849f4e179c05f2752`
- Environment: local Windows development host, local MySQL 5.7 database only.
- Production: not accessed, not migrated, not deployed.

## Confirmed Business Rules

- Base exchange rule: 1 top-up unit equals 1 USD of SDKMAX credit value.
- Stripe settlement currency: fixed USD.
- `StripeUnitPrice`: 1.
- Marketing discount rule: discount reduces Stripe charge only and does not reduce SDKMAX credited value.
- Verified discount example: 100 units -> Stripe charged 90.00 USD -> SDKMAX credited 100 USD equivalent quota.
- Stripe subscriptions remain disabled for this phase.
- Stripe promotion codes remain disabled for this phase.
- Production frontend requirement remains the default theme only.

## Local Test Mode Payment Evidence

All identifiers below are intentionally masked.

### Successful non-discount Stripe payment

- A prior 5 USD Test Mode Checkout payment completed locally.
- `checkout.session.completed` was processed by SDKMAX.
- Local order reached `success`.
- `paid_amount_minor` matched the real Stripe paid amount.
- User balance increased once.
- Webhook replay/idempotency checks showed no duplicate crediting.

### Successful 90% marketing-discount payment

- Order: `ref_9ace...1ea4`
- Session: `cs_tes...Z832`
- Stripe event: masked, `checkout.session.completed`
- Stripe state: `complete / paid`
- PaymentIntent state: `succeeded`
- Stripe paid amount: `9000` minor units, i.e. `90.00 USD`
- SDKMAX credited quota: `50000000`
- Local order state: `success`
- `stripe_event_id`: saved
- `stripe_payment_intent_id`: saved
- User balance changed from `262479434` to `312479434`
- Balance delta: `50000000`
- Top-up log count changed from `6` to `7`

This confirms the agreed rule: 100 requested units with a 90% marketing discount charges 90.00 USD while crediting 100 USD equivalent SDKMAX quota.

## Failed Card Result

- Failed Stripe Test Mode card attempts remained unpaid.
- Local pending/failed records kept `paid_amount_minor=0`.
- No user balance increase was observed for failed payment attempts.
- Non-target Stripe events such as `charge.failed` and `payment_intent.payment_failed` are handled as non-crediting events.

## Paid/Pending Safe Recovery

Several local orders became Stripe `paid/succeeded` while SDKMAX remained `pending` because local Stripe CLI was previously misconfigured or disconnected.

Confirmed causes during local testing:

- Incorrect `--events` argument using spaces instead of comma-separated values.
- Stripe CLI listening under a different sandbox account from the SDKMAX `sk_test` account.
- Stripe CLI websocket disconnect during testing.

Safe recovery was tested by using real Stripe Test Mode evidence plus a valid locally configured webhook signing secret, then posting a signed `checkout.session.completed` event to the local webhook endpoint. No direct `users` or `top_ups` update was used.

Recovered examples:

- `ref_eff3...e34e`: `pending -> success`, `paid_amount_minor=9000`, quota delta `50000000`.
- `ref_9ace...1ea4`: `pending -> success`, `paid_amount_minor=9000`, quota delta `50000000`.

Old local pending orders are retained as historical local test records. They were not completed through the ordinary admin manual supplement button and were not directly modified.

## Webhook Idempotency

Verified locally with signed Test Mode events:

- Same Event ID delivered twice: both requests returned controlled 2xx responses; balance changed once.
- Same Session ID with different Event IDs: no duplicate crediting.
- Concurrent valid callbacks: final balance changed once; one successful local crediting result.
- Historical failed 5 USD pending order remained unpaid and was not restored by unrelated replay/concurrency tests.

The model-level MySQL concurrency test also covers the following at 8-way concurrency:

- Stripe same order, same Event ID.
- Stripe same Session, different Event IDs.
- Same Event ID across different orders.
- Epay-style manual completion.
- Waffo callback.
- Creem callback.
- Waffo Pancake callback.
- Redemption code redemption.
- Subscription order completion.
- AffQuota transfer.

All covered scenarios completed without deadlock or lock wait timeout in the latest run.

## Display Fix Validation

The default frontend now uses `getPaidAmountDisplay` for the billing history paid amount column:

- Stripe `success` records display the actual `paid_amount_minor`.
- Stripe `pending`, `failed`, and `expired` records display `Unpaid` / `未支付`.
- Non-Stripe records continue using the legacy `money` display path.

This prevents unpaid Stripe orders from showing `expected_amount_minor` as if it were already paid.

## Actual Commands And Results

- `git branch --show-current`: `feature/stripe-one-time-topup-audit-fixes`
- `git rev-parse HEAD`: `8b0010f9fd81787d2bfa656849f4e179c05f2752`
- `git status --short`: showed only the Stripe display fix files, new display test files, and unrelated untracked user documentation directories.
- `git diff --stat`: 7 tracked frontend files changed before adding new files.
- `git diff --check`: passed.
- `node --test src/features/wallet/lib/payment-display.test.ts`: passed, 4 tests.
- `npm run build`: passed.
- `D:\sdkmax\tools\go\bin\go.exe test ./controller -run TestStripe -v`: passed.
- `D:\sdkmax\tools\go\bin\go.exe test ./model -run 'TestCompleteStripeTopUp|TestStripe|TestTopUpSchema|TestMarkStripe|TestManualCompleteTopUp' -v`: passed.
- `D:\sdkmax\tools\go\bin\go.exe test ./model/... -run TestMySQLConcurrency -v -count=1`: passed against local `sdkmax_concurrency_test`.
- `npm run typecheck`: failed due existing unrelated `src/features/usage-logs/components/usage-logs-mobile-card.tsx` type errors for `created_at` and `type`.
- `npm run lint`: failed due existing ESLint/minimatch runtime error: `brace_expansion_1.expand is not a function`.

## Untracked User Directories

The following pre-existing untracked directories were listed but not modified or staged:

- `docs/CODE_REVIEW/`
- `docs/deployment/`
- `docs/legal/`

## Secrets And Privacy Check

No real Stripe API credential, webhook credential, request signature, complete provider payload, payment card details, full Stripe object identifier, full database connection string, database password, local environment file, local database file, or Stripe CLI terminal output is intended to be included in this commit.

All report evidence uses masked IDs.

## Unverified Items

- Production migration execution: not run.
- Production Stripe Live Mode configuration: not created.
- Production webhook endpoint: not created.
- GitHub Actions validation for the new local commit: not run yet.
- Production release merge and deployment: not performed.

## Production Safety Statement

This validation did not access production servers, did not connect to production databases, did not execute production SQL, did not configure `sk_live_`, did not create a production webhook, did not create a Stripe Live payment, did not deploy, and did not merge release branches.

## Readiness Conclusion

Local Stripe Test Mode validation is sufficient to create an auditable local commit and prepare CI verification. The next step after local commit is to push the feature branch only when approved, then run GitHub Actions build and test verification before any release preparation.
