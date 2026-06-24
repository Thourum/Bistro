// Bistro UI kit — Login screen. Mirrors Bistro-Login (Minimal).
const { Eyebrow } = window.BistroDesignSystem_8e22c3;

function ProviderRow({ icon, label, keyHint, onClick }) {
  const [hover, setHover] = React.useState(false);
  return (
    <button
      type="button"
      onClick={onClick}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        width: "100%",
        padding: "0 20px",
        height: 64,
        cursor: "pointer",
        background: hover
          ? "var(--color-surface-container-low)"
          : "var(--color-surface-container-lowest)",
        border: `1px solid ${hover ? "var(--color-primary)" : "var(--color-outline-variant)"}`,
        transition: "background-color 120ms, border-color 120ms",
      }}
    >
      <span style={{ display: "flex", alignItems: "center", gap: 16 }}>
        <span
          className="material-symbols-outlined"
          style={{ fontSize: 26, color: "var(--color-on-surface)" }}
        >
          {icon}
        </span>
        <span
          style={{
            fontFamily: "var(--font-sans)",
            fontSize: 18,
            fontWeight: 700,
            color: "var(--color-on-surface)",
          }}
        >
          {label}
        </span>
      </span>
      <span style={{ display: "flex", alignItems: "center", gap: 10 }}>
        <kbd
          style={{
            fontFamily: "var(--font-mono)",
            fontSize: 11,
            padding: "3px 9px",
            border: "1px solid var(--color-outline-variant)",
            color: "var(--color-on-surface-variant)",
          }}
        >
          {keyHint}
        </kbd>
        <span
          style={{
            fontFamily: "var(--font-mono)",
            fontSize: 10,
            letterSpacing: "0.12em",
            textTransform: "uppercase",
            color: "var(--color-outline)",
          }}
        >
          Key hint
        </span>
      </span>
    </button>
  );
}

function LoginScreen({ onLogin }) {
  return (
    <div
      style={{
        height: "100%",
        display: "flex",
        flexDirection: "column",
        background: "var(--color-surface)",
      }}
    >
      <div
        style={{
          flex: 1,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        <div
          style={{
            width: 588,
            background: "var(--color-surface-container-lowest)",
            boxShadow: "var(--shadow-ambient)",
            padding: 48,
          }}
        >
          <div style={{ display: "flex", alignItems: "center", gap: 18 }}>
            <img
              src="../../assets/bistro-mark.svg"
              width="58"
              height="58"
              alt=""
            />
            <h1
              style={{
                margin: 0,
                fontFamily: "var(--font-sans)",
                fontSize: 52,
                fontWeight: 800,
                letterSpacing: "-0.04em",
                textTransform: "uppercase",
                color: "var(--color-on-surface)",
              }}
            >
              Bistro
            </h1>
          </div>
          <p
            style={{
              margin: "16px 0 40px",
              fontFamily: "var(--font-sans)",
              fontSize: 16,
              color: "var(--color-on-surface-variant)",
            }}
          >
            Connect your platform to begin reviewing code
          </p>
          <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
            <ProviderRow
              icon="terminal"
              label="GitHub"
              keyHint="G"
              onClick={onLogin}
            />
            <ProviderRow
              icon="folder_open"
              label="GitLab"
              keyHint="L"
              onClick={onLogin}
            />
            <ProviderRow
              icon="local_cafe"
              label="Gitea"
              keyHint="T"
              onClick={onLogin}
            />
          </div>
          <div
            style={{
              marginTop: 56,
              display: "flex",
              alignItems: "flex-end",
              justifyContent: "space-between",
            }}
          >
            <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
              <Eyebrow
                style={{ fontSize: 10, color: "var(--color-outline-variant)" }}
              >
                Status:
              </Eyebrow>
              <Eyebrow
                style={{ fontSize: 10, color: "var(--color-outline-variant)" }}
              >
                Ready
              </Eyebrow>
              <span
                style={{
                  fontFamily: "var(--font-mono)",
                  fontSize: 10,
                  color: "var(--color-outline-variant)",
                  textTransform: "none",
                }}
              >
                v4.0.2-stable
              </span>
            </div>
            <div
              style={{
                width: 96,
                height: 1,
                background: "var(--color-outline-variant)",
              }}
            />
          </div>
        </div>
      </div>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          height: 40,
          padding: "0 24px",
          borderTop: "1px solid var(--color-outline-variant)",
          background: "var(--color-surface-container-high)",
        }}
      >
        <Eyebrow style={{ fontSize: 11 }}>
          Bistro // Technical Atelier&nbsp;&nbsp;© 2026
        </Eyebrow>
        <div style={{ marginLeft: "auto", display: "flex", gap: 18 }}>
          <Eyebrow
            style={{ fontSize: 11, color: "var(--color-outline-variant)" }}
          >
            Documentation
          </Eyebrow>
          <Eyebrow
            style={{ fontSize: 11, color: "var(--color-outline-variant)" }}
          >
            Support
          </Eyebrow>
          <Eyebrow
            style={{ fontSize: 11, color: "var(--color-outline-variant)" }}
          >
            Status
          </Eyebrow>
        </div>
      </div>
    </div>
  );
}

window.LoginScreen = LoginScreen;
