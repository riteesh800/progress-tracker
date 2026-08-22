type PasswordFieldProps = {
  label: string;
  value: string;
  onChange: (value: string) => void;
  visible: boolean;
  onToggle: () => void;
  required?: boolean;
  minLength?: number;
  autoComplete?: string;
};

export default function PasswordField({
  label,
  value,
  onChange,
  visible,
  onToggle,
  required,
  minLength = 8,
  autoComplete,
}: PasswordFieldProps) {
  return (
    <label className="pw-field">
      {label}
      <span className="pw-wrap">
        <input
          value={value}
          onChange={(e) => onChange(e.target.value)}
          type={visible ? "text" : "password"}
          minLength={minLength}
          required={required}
          autoComplete={autoComplete}
        />
        <button
          type="button"
          className="pw-toggle"
          onClick={onToggle}
          aria-label={visible ? "Hide password" : "Show password"}
          title={visible ? "Hide password" : "Show password"}
        >
          {visible ? (
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden>
              <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" />
              <circle cx="12" cy="12" r="3" />
            </svg>
          ) : (
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden>
              <path d="M3 3l18 18" />
              <path d="M10.6 10.6A3 3 0 0 0 12 15a3 3 0 0 0 2.4-1.2" />
              <path d="M9.9 5.1A11 11 0 0 1 12 5c6.5 0 10 7 10 7a18 18 0 0 1-4.1 4.8" />
              <path d="M6.1 6.1A18 18 0 0 0 2 12s3.5 7 10 7a11 11 0 0 0 3.4-.5" />
            </svg>
          )}
        </button>
      </span>
    </label>
  );
}
