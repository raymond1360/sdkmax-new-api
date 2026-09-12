/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { TopupRecord } from '../types'

export type PaidAmountDisplay =
  | {
      kind: 'paid_minor'
      amountMinor: number
      currency: string
    }
  | {
      kind: 'unpaid'
    }
  | {
      kind: 'legacy_money'
      money: number
    }

type PaidAmountRecord = {
  currency?: TopupRecord['currency']
  money: TopupRecord['money']
  paid_amount_minor?: TopupRecord['paid_amount_minor']
  status: TopupRecord['status']
}

export function getPaidAmountDisplay(
  record: PaidAmountRecord
): PaidAmountDisplay {
  if (!record.currency) {
    return {
      kind: 'legacy_money',
      money: record.money,
    }
  }

  if (
    record.status === 'success' &&
    typeof record.paid_amount_minor === 'number' &&
    record.paid_amount_minor > 0
  ) {
    return {
      kind: 'paid_minor',
      amountMinor: record.paid_amount_minor,
      currency: record.currency,
    }
  }

  return {
    kind: 'unpaid',
  }
}
