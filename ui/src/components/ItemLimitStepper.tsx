
interface ItemLimitStepperProps {
  current: number
  total: number
  min?: number
  max?: number
  onChange: (value: number) => void
}

export default function ItemLimitStepper({
  current,
  total,
  min = 1,
  max = 20,
  onChange,
}: ItemLimitStepperProps) {
  if (total <= current) return null

  return (
    <div className="item-limit-stepper">
      <button
        type="button"
        className="item-limit-stepper-btn"
        onClick={() => onChange(Math.max(min, current - 1))}
        disabled={current <= min}
      >
        &minus;
      </button>
      <span className="item-limit-stepper-label">
        {current} / {total}
      </span>
      <button
        type="button"
        className="item-limit-stepper-btn"
        onClick={() => onChange(Math.min(max, current + 1))}
        disabled={current >= max}
      >
        +
      </button>
    </div>
  )
}
