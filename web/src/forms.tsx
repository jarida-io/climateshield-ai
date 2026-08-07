// SPDX-License-Identifier: Apache-2.0

// Form controls for the dashboard.
//
// Every control here QUERIES or PREVIEWS. None of them writes: the public tier
// is read-only by design, and a write path on an unauthenticated public
// surface would breach that. Nothing in this file posts user input to the
// server either — the one control that accepts free text (the message
// previewer) renders entirely in the browser, so a name typed into it is never
// transmitted, logged or stored.

import type { ReactNode } from "react";
import { useId } from "react";

import { brand, space, text } from "./ui";

const controlStyle = {
  ...text.body,
  fontFamily: "inherit",
  padding: `${space(2)} ${space(3)}`,
  border: `1px solid ${brand.line}`,
  borderRadius: 8,
  background: brand.surface,
  color: brand.ink,
  minWidth: 0,
} as const;

/** A labelled control. The label is always present and always associated. */
export function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: (id: string) => ReactNode;
}) {
  const id = useId();
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: space(1), minWidth: 150 }}>
      <label htmlFor={id} style={{ ...text.small, color: brand.muted, fontWeight: 700 }}>
        {label}
      </label>
      {children(id)}
      {hint !== undefined && (
        <span style={{ ...text.small, color: brand.muted }}>{hint}</span>
      )}
    </div>
  );
}

export function Select({
  label,
  value,
  options,
  onChange,
  hint,
}: {
  label: string;
  value: string;
  options: { value: string; label: string }[];
  onChange: (v: string) => void;
  hint?: string;
}) {
  return (
    <Field label={label} {...(hint === undefined ? {} : { hint })}>
      {(id) => (
        <select id={id} value={value} onChange={(e) => onChange(e.target.value)} style={controlStyle}>
          {options.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      )}
    </Field>
  );
}

export function TextInput({
  label,
  value,
  onChange,
  hint,
  maxLength,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  hint?: string;
  maxLength?: number;
  placeholder?: string;
}) {
  return (
    <Field label={label} {...(hint === undefined ? {} : { hint })}>
      {(id) => (
        <input
          id={id}
          type="text"
          value={value}
          maxLength={maxLength ?? 40}
          placeholder={placeholder ?? ""}
          onChange={(e) => onChange(e.target.value)}
          style={controlStyle}
        />
      )}
    </Field>
  );
}

/** A row of filters, sitting above the charts they control. */
export function FilterBar({ children }: { children: ReactNode }) {
  return (
    <form
      onSubmit={(e) => e.preventDefault()}
      style={{
        display: "flex",
        gap: space(4),
        flexWrap: "wrap",
        alignItems: "flex-end",
        padding: space(4),
        background: brand.surface,
        border: `1px solid ${brand.line}`,
        borderRadius: 10,
        marginBottom: space(5),
      }}
    >
      {children}
    </form>
  );
}

export function Button({
  children,
  onClick,
  kind = "secondary",
}: {
  children: ReactNode;
  onClick: () => void;
  kind?: "primary" | "secondary";
}) {
  const primary = kind === "primary";
  return (
    <button
      type="button"
      onClick={onClick}
      style={{
        ...text.body,
        fontFamily: "inherit",
        fontWeight: 700,
        padding: `${space(2)} ${space(4)}`,
        borderRadius: 8,
        cursor: "pointer",
        border: `1px solid ${primary ? brand.purple : brand.line}`,
        background: primary ? brand.purple : brand.surface,
        color: primary ? "#fff" : brand.ink,
      }}
    >
      {children}
    </button>
  );
}

/** A checkbox that reads as a toggle. */
export function Toggle({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  const id = useId();
  return (
    <div style={{ display: "flex", alignItems: "center", gap: space(2), paddingBottom: space(2) }}>
      <input
        id={id}
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        style={{ width: 16, height: 16, accentColor: brand.purple }}
      />
      <label htmlFor={id} style={{ ...text.body, color: brand.ink }}>
        {label}
      </label>
    </div>
  );
}
