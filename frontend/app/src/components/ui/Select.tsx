import { useEffect, useId, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import "./Select.css";

export type SelectOption = {
  value: string;
  label: ReactNode;
  disabled?: boolean;
  text?: string;
};

export type SelectProps = {
  options: SelectOption[];
  value?: string;
  defaultValue?: string;
  onChange?: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  required?: boolean;
  name?: string;
  id?: string;
  className?: string;
  "aria-label"?: string;
  "aria-labelledby"?: string;
  "aria-describedby"?: string;
};

const TYPEAHEAD_TIMEOUT = 500;

function firstEnabledIndex(options: SelectOption[]) {
  return options.findIndex((option) => !option.disabled);
}

function nextEnabledIndex(options: SelectOption[], start: number, direction: 1 | -1) {
  if (options.length === 0) return -1;

  let index = start;
  for (let count = 0; count < options.length; count += 1) {
    index = (index + direction + options.length) % options.length;
    if (!options[index].disabled) return index;
  }
  return -1;
}

export default function Select({
  options,
  value,
  defaultValue = "",
  onChange,
  placeholder = "Select an option",
  disabled = false,
  required = false,
  name,
  id,
  className,
  "aria-label": ariaLabel,
  "aria-labelledby": ariaLabelledBy,
  "aria-describedby": ariaDescribedBy,
}: SelectProps) {
  const generatedId = useId();
  const selectId = id ?? `select-${generatedId}`;
  const listboxId = `${selectId}-listbox`;
  const triggerRef = useRef<HTMLButtonElement>(null);
  const optionRefs = useRef<Array<HTMLDivElement | null>>([]);
  const wasOpenRef = useRef(false);
  const typeaheadRef = useRef("");
  const typeaheadTimerRef = useRef<number | undefined>(undefined);
  const isControlled = value !== undefined;
  const [internalValue, setInternalValue] = useState(defaultValue);
  const [open, setOpen] = useState(false);
  const selectedValue = isControlled ? value : internalValue;
  const selectedIndex = options.findIndex((option) => option.value === selectedValue);
  const [activeIndex, setActiveIndex] = useState(() =>
    selectedIndex >= 0 && !options[selectedIndex].disabled ? selectedIndex : firstEnabledIndex(options),
  );
  const selectedOption = options[selectedIndex];
  const enabledOptions = options.filter((option) => !option.disabled);
  const displayValue = selectedOption?.label ?? placeholder;

  useEffect(() => {
    if (!open) {
      wasOpenRef.current = false;
      return;
    }
    if (wasOpenRef.current) return;
    wasOpenRef.current = true;
    const nextIndex = selectedIndex >= 0 && !options[selectedIndex].disabled ? selectedIndex : firstEnabledIndex(options);
    setActiveIndex(nextIndex);
    if (nextIndex >= 0) optionRefs.current[nextIndex]?.focus();
  }, [open, options, selectedIndex]);

  useEffect(() => {
    if (!open) return;

    const closeOnOutsidePointer = (event: PointerEvent) => {
      if (!(event.target instanceof Node) || event.target === triggerRef.current || triggerRef.current?.contains(event.target)) return;
      if (!optionRefs.current.some((option) => option?.contains(event.target as Node))) setOpen(false);
    };

    document.addEventListener("pointerdown", closeOnOutsidePointer);
    return () => document.removeEventListener("pointerdown", closeOnOutsidePointer);
  }, [open]);

  useEffect(() => () => {
    if (typeaheadTimerRef.current !== undefined) window.clearTimeout(typeaheadTimerRef.current);
  }, []);

  function focusOption(index: number) {
    if (index < 0 || options[index]?.disabled) return;
    setActiveIndex(index);
    optionRefs.current[index]?.focus();
  }

  function choose(option: SelectOption) {
    if (option.disabled) return;
    if (!isControlled) setInternalValue(option.value);
    onChange?.(option.value);
    setOpen(false);
    triggerRef.current?.focus();
  }

  function openListbox() {
    if (disabled || enabledOptions.length === 0) return;
    setOpen(true);
  }

  function moveActive(direction: 1 | -1) {
    const start = activeIndex >= 0 ? activeIndex : firstEnabledIndex(options);
    focusOption(nextEnabledIndex(options, start, direction));
  }

  function typeAhead(character: string) {
    const query = `${typeaheadRef.current}${character}`.toLocaleLowerCase();
    typeaheadRef.current = query;
    if (typeaheadTimerRef.current !== undefined) window.clearTimeout(typeaheadTimerRef.current);
    typeaheadTimerRef.current = window.setTimeout(() => {
      typeaheadRef.current = "";
    }, TYPEAHEAD_TIMEOUT);

    const currentEnabledIndex = enabledOptions.findIndex((option) => options.indexOf(option) === activeIndex);
    for (let offset = 1; offset <= enabledOptions.length; offset += 1) {
      const option = enabledOptions[(currentEnabledIndex + offset + enabledOptions.length) % enabledOptions.length];
      if ((option.text ?? String(option.label)).toLocaleLowerCase().startsWith(query)) {
        focusOption(options.indexOf(option));
        break;
      }
    }
  }

  function handleTriggerKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    if (disabled) return;
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      if (open) choose(options[activeIndex]);
      else openListbox();
    } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      if (!open) openListbox();
      else moveActive(event.key === "ArrowDown" ? 1 : -1);
    } else if (event.key === "Escape" && open) {
      event.preventDefault();
      setOpen(false);
    }
  }

  function handleOptionKeyDown(event: KeyboardEvent<HTMLDivElement>, index: number) {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      moveActive(event.key === "ArrowDown" ? 1 : -1);
    } else if (event.key === "Home" || event.key === "End") {
      event.preventDefault();
      const target = event.key === "Home" ? firstEnabledIndex(options) : options.findLastIndex((option) => !option.disabled);
      focusOption(target);
    } else if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      choose(options[index]);
    } else if (event.key === "Escape") {
      event.preventDefault();
      setOpen(false);
      triggerRef.current?.focus();
    } else if (event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
      typeAhead(event.key);
    }
  }

  return (
    <div className={`select ${className ?? ""}`.trim()}>
      {name && <input type="hidden" name={name} value={selectedValue} required={required} />}
      <button
        ref={triggerRef}
        id={selectId}
        className="select-trigger"
        type="button"
        disabled={disabled}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={listboxId}
        aria-label={ariaLabel}
        aria-labelledby={ariaLabelledBy}
        aria-describedby={ariaDescribedBy}
        aria-required={required || undefined}
        onClick={() => (open ? setOpen(false) : openListbox())}
        onKeyDown={handleTriggerKeyDown}
      >
        <span className={selectedOption ? "select-value" : "select-placeholder"}>{displayValue}</span>
        <span className="select-caret" aria-hidden="true">↓</span>
      </button>
      {open && (
        <div id={listboxId} className="select-listbox" role="listbox" aria-labelledby={ariaLabelledBy ?? selectId}>
          {options.map((option, index) => (
            <div
              key={option.value}
              ref={(element) => { optionRefs.current[index] = element; }}
              className="select-option"
              id={`${listboxId}-option-${index}`}
              role="option"
              aria-selected={option.value === selectedValue}
              aria-disabled={option.disabled || undefined}
              tabIndex={option.disabled ? -1 : index === activeIndex ? 0 : -1}
              onClick={() => choose(option)}
              onKeyDown={(event) => handleOptionKeyDown(event, index)}
            >
              {option.label}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
