import React from 'react';
import { useNavigate } from 'react-router-dom';
import './Landing.css';

/* ── Subtle geometric background ──────────────────────────────────────────── */
function BgSvg() {
  return (
    <svg className="landing-svg-bg" viewBox="0 0 1440 900" preserveAspectRatio="xMidYMid slice" xmlns="http://www.w3.org/2000/svg">
      {/* Ambient radial glow — top center */}
      <defs>
        <radialGradient id="glow-top" cx="50%" cy="0%" r="55%">
          <stop offset="0%" stopColor="#00b4d8" stopOpacity="0.09"/>
          <stop offset="100%" stopColor="#00b4d8" stopOpacity="0"/>
        </radialGradient>
        <radialGradient id="glow-br" cx="100%" cy="100%" r="40%">
          <stop offset="0%" stopColor="#00b4d8" stopOpacity="0.05"/>
          <stop offset="100%" stopColor="#00b4d8" stopOpacity="0"/>
        </radialGradient>
      </defs>
      <rect width="1440" height="900" fill="url(#glow-top)"/>
      <rect width="1440" height="900" fill="url(#glow-br)"/>

      {/* Fine dot grid */}
      <pattern id="dots" width="36" height="36" patternUnits="userSpaceOnUse">
        <circle cx="1" cy="1" r="1" fill="#8ba3bf" opacity="0.12"/>
      </pattern>
      <rect width="1440" height="900" fill="url(#dots)"/>

      {/* Flowing line traces — left */}
      {[
        { d: 'M 0 320 H 60 V 400 H 120 V 460', delay: '0s',   dur: '4s' },
        { d: 'M 0 500 H 40 V 470 H 100 V 540 H 160', delay: '1.2s', dur: '5s' },
        { d: 'M 0 680 H 80 V 640 H 140', delay: '0.6s', dur: '3.5s' },
      ].map((t, i) => (
        <path key={i} d={t.d} fill="none" stroke="#00b4d8" strokeWidth="0.8"
          strokeDasharray="180" strokeLinecap="round" opacity="0.25"
          style={{ animation: `line-flow ${t.dur} ${t.delay} linear infinite` }}
        />
      ))}
      {/* Trace nodes */}
      {[[60,320],[120,400],[40,500],[100,470],[160,540],[80,680],[140,640]].map(([cx,cy],i) => (
        <circle key={i} cx={cx} cy={cy} r="2" fill="#00b4d8" opacity="0.3"
          style={{ animation: `node-glow ${2+(i%3)}s ${i*0.4}s ease-in-out infinite` }}
        />
      ))}

      {/* Flowing line traces — right */}
      {[
        { d: 'M 1440 260 H 1380 V 340 H 1320', delay: '0.8s', dur: '4.2s' },
        { d: 'M 1440 460 H 1400 V 420 H 1330 V 500 H 1270', delay: '0s',   dur: '3.8s' },
        { d: 'M 1440 660 H 1360 V 620 H 1300', delay: '1.5s', dur: '4.5s' },
      ].map((t, i) => (
        <path key={i} d={t.d} fill="none" stroke="#00b4d8" strokeWidth="0.8"
          strokeDasharray="180" strokeLinecap="round" opacity="0.25"
          style={{ animation: `line-flow ${t.dur} ${t.delay} linear infinite` }}
        />
      ))}

      {/* Subtle corner accents */}
      <polyline points="40,36 20,36 20,64" fill="none" stroke="#00b4d8" strokeWidth="1.5" opacity="0.2"/>
      <polyline points="1400,36 1420,36 1420,64" fill="none" stroke="#00b4d8" strokeWidth="1.5" opacity="0.2"/>
      <polyline points="40,864 20,864 20,836" fill="none" stroke="#00b4d8" strokeWidth="1.5" opacity="0.2"/>
      <polyline points="1400,864 1420,864 1420,836" fill="none" stroke="#00b4d8" strokeWidth="1.5" opacity="0.2"/>

      {/* Horizontal rule under nav area */}
      <line x1="0" y1="72" x2="1440" y2="72" stroke="#8ba3bf" strokeWidth="0.5" opacity="0.1"/>
    </svg>
  );
}

/* ── Feature icons ──────────────────────────────────────────────────────── */
function IconChat() {
  return (
    <svg className="landing-feature-icon" viewBox="0 0 40 40" fill="none" xmlns="http://www.w3.org/2000/svg">
      <rect x="4" y="6" width="24" height="18" rx="4" stroke="#00b4d8" strokeWidth="1.6"/>
      <path d="M4 20 L4 26 L10 20" fill="#00b4d8" opacity="0.5"/>
      <rect x="14" y="18" width="22" height="16" rx="4" fill="#0d1f3c" stroke="#00b4d8" strokeWidth="1.4"/>
      <line x1="19" y1="24" x2="31" y2="24" stroke="#00b4d8" strokeWidth="1.2" opacity="0.6" strokeLinecap="round"/>
      <line x1="19" y1="28" x2="27" y2="28" stroke="#00b4d8" strokeWidth="1.2" opacity="0.4" strokeLinecap="round"/>
    </svg>
  );
}

function IconLock() {
  return (
    <svg className="landing-feature-icon" viewBox="0 0 40 40" fill="none" xmlns="http://www.w3.org/2000/svg">
      <rect x="9" y="18" width="22" height="16" rx="3" stroke="#00b4d8" strokeWidth="1.6"/>
      <path d="M14 18 V13 A6 6 0 0 1 26 13 V18" stroke="#00b4d8" strokeWidth="1.6"/>
      <circle cx="20" cy="26" r="2.5" fill="#00b4d8" opacity="0.8"/>
      <line x1="20" y1="28.5" x2="20" y2="31" stroke="#00b4d8" strokeWidth="1.5" strokeLinecap="round"/>
    </svg>
  );
}

function IconServer() {
  return (
    <svg className="landing-feature-icon" viewBox="0 0 40 40" fill="none" xmlns="http://www.w3.org/2000/svg">
      <rect x="6" y="8" width="28" height="8" rx="3" stroke="#00b4d8" strokeWidth="1.5"/>
      <circle cx="30" cy="12" r="2" fill="#00b4d8" opacity="0.7"/>
      <rect x="6" y="20" width="28" height="8" rx="3" stroke="#00b4d8" strokeWidth="1.5"/>
      <circle cx="30" cy="24" r="2" fill="#00b4d8" opacity="0.5"/>
      <rect x="6" y="32" width="28" height="8" rx="3" stroke="#00b4d8" strokeWidth="1.5" opacity="0.5"/>
      <circle cx="30" cy="36" r="2" fill="#00b4d8" opacity="0.3"/>
    </svg>
  );
}

/* ── Landing Page ────────────────────────────────────────────────────────── */
export const Landing: React.FC = () => {
  const navigate = useNavigate();

  return (
    <div className="landing">
      <BgSvg />

      <div className="landing-content">
        {/* Nav */}
        <nav className="landing-nav">
          <span className="landing-nav-brand">Parley</span>
          <div className="landing-nav-links">
            <button className="landing-nav-link" onClick={() => navigate('/login')}>Sign In</button>
            <button className="landing-nav-link landing-nav-link-cta" onClick={() => navigate('/register')}>Get Started</button>
          </div>
        </nav>

        {/* Hero */}
        <section className="landing-hero">
          <div className="landing-logo-wrap">
            <img src="/logo.svg" alt="Parley" className="landing-logo" width="420" height="100" />
          </div>

          <div className="landing-tagline">
            Where your community connects
          </div>
          <div className="landing-tagline-sub">
            Real-time messaging, voice, and collaboration — built for every team
          </div>

          <div className="landing-ctas">
            <button className="landing-btn landing-btn-primary" onClick={() => navigate('/register')}>
              Create an Account
            </button>
            <button className="landing-btn landing-btn-secondary" onClick={() => navigate('/login')}>
              Sign In
            </button>
          </div>

          {/* Divider */}
          <div className="landing-divider">
            <div className="landing-divider-line" />
            <span className="landing-divider-text">Everything you need</span>
            <div className="landing-divider-line" />
          </div>

          {/* Features */}
          <div className="landing-features">
            <div className="landing-feature">
              <IconServer />
              <div className="landing-feature-title">Servers &amp; Channels</div>
              <div className="landing-feature-desc">
                Organize your community into servers with dedicated channels for every topic. Text, voice, and everything in between.
              </div>
            </div>
            <div className="landing-feature">
              <IconLock />
              <div className="landing-feature-title">Secure by Default</div>
              <div className="landing-feature-desc">
                TLS on every connection. Role-based permissions let you control exactly who sees what, down to the channel level.
              </div>
            </div>
            <div className="landing-feature">
              <IconChat />
              <div className="landing-feature-title">Direct Messages</div>
              <div className="landing-feature-desc">
                Private one-on-one conversations. Files, images, code snippets with syntax highlighting — the full toolkit.
              </div>
            </div>
          </div>
        </section>

        {/* Footer */}
        <footer className="landing-footer">
          <span>© 2026 Parley</span>
          <span>Built for real-time communication</span>
        </footer>
      </div>
    </div>
  );
};
