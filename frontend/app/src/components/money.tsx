import { formatMoney } from "../lib/money";
import "./money.css";

type MoneyProps = {
  value: number | bigint;
  currency: string;
  className?: string;
};

export default function Money({ value, currency, className }: MoneyProps) {
  return <span className={["money", className].filter(Boolean).join(" ")}>{formatMoney(value, currency)}</span>;
}
