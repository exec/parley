import React, { useEffect, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import './Landing.css';

/* ── Matrix Rain ─────────────────────────────────────────────────────────── */
const RAIN_CHARS = 'ABCDEF0123456789!@#$%^&*<>/\\|~アイウエオカキクケコサシスセソ死骨海賊暗号PARLEY';

function startRain(canvas: HTMLCanvasElement): () => void {
  const ctx = canvas.getContext('2d')!;
  let W = 0, H = 0;
  let cols: number[] = [];
  let raf = 0;

  const resize = () => {
    W = canvas.width  = window.innerWidth;
    H = canvas.height = window.innerHeight;
    const colCount = Math.floor(W / 18);
    cols = Array.from({ length: colCount }, () => Math.random() * -H);
  };
  resize();
  window.addEventListener('resize', resize);

  const draw = () => {
    ctx.fillStyle = 'rgba(0,0,0,0.055)';
    ctx.fillRect(0, 0, W, H);

    for (let i = 0; i < cols.length; i++) {
      const bright = Math.random() > 0.96;
      ctx.fillStyle = bright ? '#aaffaa' : '#32CD32';
      ctx.font = `${12 + Math.floor(Math.random() * 5)}px "Courier New", monospace`;
      const ch = RAIN_CHARS[Math.floor(Math.random() * RAIN_CHARS.length)];
      ctx.fillText(ch, i * 18, cols[i]);

      if (cols[i] > H && Math.random() > 0.975) {
        cols[i] = 0;
      } else {
        cols[i] += 18 + Math.random() * 6;
      }
    }
    raf = requestAnimationFrame(draw);
  };
  draw();

  return () => {
    cancelAnimationFrame(raf);
    window.removeEventListener('resize', resize);
  };
}

/* ── SVG Background ──────────────────────────────────────────────────────── */
function BgSvg() {
  return (
    <svg className="landing-svg-bg" viewBox="0 0 1440 900" preserveAspectRatio="xMidYMid slice" xmlns="http://www.w3.org/2000/svg">
      <defs>
        <filter id="lg">
          <feGaussianBlur stdDeviation="3" result="b"/>
          <feMerge><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge>
        </filter>
      </defs>

      {/* ── Ghost skull — top-left ── */}
      <g style={{ animation: 'skull-drift 11s ease-in-out infinite', transformOrigin: '160px 160px' }}>
        <ellipse cx="160" cy="140" rx="90" ry="80" fill="none" stroke="#32CD32" strokeWidth="1.5"/>
        <path d="M70 180 Q70 220 160 220 Q250 220 250 180" fill="none" stroke="#32CD32" strokeWidth="1.4"/>
        <line x1="130" y1="195" x2="130" y2="220" stroke="#32CD32" strokeWidth="1.2"/>
        <line x1="160" y1="196" x2="160" y2="222" stroke="#32CD32" strokeWidth="1.2"/>
        <line x1="190" y1="195" x2="190" y2="220" stroke="#32CD32" strokeWidth="1.2"/>
        <polygon points="137,128 122,148 137,168 152,148" fill="#32CD32"/>
        <polygon points="183,128 168,148 183,168 198,148" fill="#32CD32"/>
        <polygon points="160,148 152,162 168,162" fill="#32CD32" opacity="0.7"/>
        <line x1="80" y1="240" x2="240" y2="290" stroke="#32CD32" strokeWidth="3" strokeLinecap="round"/>
        <circle cx="80"  cy="240" r="10" fill="none" stroke="#32CD32" strokeWidth="1.5"/>
        <circle cx="240" cy="290" r="10" fill="none" stroke="#32CD32" strokeWidth="1.5"/>
        <line x1="240" y1="240" x2="80"  y2="290" stroke="#32CD32" strokeWidth="3" strokeLinecap="round"/>
        <circle cx="240" cy="240" r="10" fill="none" stroke="#32CD32" strokeWidth="1.5"/>
        <circle cx="80"  cy="290" r="10" fill="none" stroke="#32CD32" strokeWidth="1.5"/>
      </g>

      {/* ── Ghost skull — bottom-right ── */}
      <g style={{ animation: 'skull-drift 14s ease-in-out infinite reverse', transformOrigin: '1280px 740px' }}>
        <ellipse cx="1280" cy="700" rx="70" ry="65" fill="none" stroke="#32CD32" strokeWidth="1.2"/>
        <path d="M1210 745 Q1210 778 1280 778 Q1350 778 1350 745" fill="none" stroke="#32CD32" strokeWidth="1.1"/>
        <line x1="1255" y1="758" x2="1255" y2="778" stroke="#32CD32" strokeWidth="1"/>
        <line x1="1280" y1="759" x2="1280" y2="780" stroke="#32CD32" strokeWidth="1"/>
        <line x1="1305" y1="758" x2="1305" y2="778" stroke="#32CD32" strokeWidth="1"/>
        <polygon points="1258,690 1246,706 1258,722 1270,706" fill="#32CD32"/>
        <polygon points="1302,690 1290,706 1302,722 1314,706" fill="#32CD32"/>
        <line x1="1215" y1="795" x2="1345" y2="832" stroke="#32CD32" strokeWidth="2.5" strokeLinecap="round"/>
        <circle cx="1215" cy="795" r="8" fill="none" stroke="#32CD32" strokeWidth="1.2"/>
        <circle cx="1345" cy="832" r="8" fill="none" stroke="#32CD32" strokeWidth="1.2"/>
        <line x1="1345" y1="795" x2="1215" y2="832" stroke="#32CD32" strokeWidth="2.5" strokeLinecap="round"/>
        <circle cx="1345" cy="795" r="8" fill="none" stroke="#32CD32" strokeWidth="1.2"/>
        <circle cx="1215" cy="832" r="8" fill="none" stroke="#32CD32" strokeWidth="1.2"/>
      </g>

      {/* ── Circuit traces — left panel ── */}
      {[
        { d: 'M 20 300 H 80 V 380 H 140', delay: '0s', dur: '3s' },
        { d: 'M 20 450 H 60 V 420 H 110 V 500 H 180', delay: '0.8s', dur: '4s' },
        { d: 'M 20 600 H 90 V 560 H 160 V 620', delay: '1.6s', dur: '3.5s' },
        { d: 'M 20 720 H 70 V 700 H 130', delay: '0.3s', dur: '5s' },
      ].map((t, i) => (
        <path
          key={i}
          d={t.d}
          fill="none"
          stroke="#32CD32"
          strokeWidth="1"
          strokeDasharray="200"
          strokeLinecap="square"
          style={{ animation: `circuit-flow ${t.dur} ${t.delay} linear infinite` }}
        />
      ))}
      {/* Circuit nodes */}
      {[
        [80, 300], [140, 380], [60, 450], [110, 420], [180, 500],
        [90, 600], [160, 560], [70, 720], [130, 700],
      ].map(([cx, cy], i) => (
        <circle key={i} cx={cx} cy={cy} r="2.5" fill="#32CD32"
          style={{ animation: `node-pulse ${2 + (i % 3)}s ${i * 0.3}s ease-in-out infinite` }}
        />
      ))}

      {/* ── Circuit traces — right panel ── */}
      {[
        { d: 'M 1420 280 H 1360 V 360 H 1300', delay: '0.5s', dur: '3.8s' },
        { d: 'M 1420 480 H 1380 V 440 H 1310 V 520 H 1250', delay: '1.2s', dur: '4.2s' },
        { d: 'M 1420 640 H 1350 V 600 H 1280', delay: '0s',   dur: '3.2s' },
        { d: 'M 1420 760 H 1370 V 740 H 1300 V 800', delay: '2s', dur: '5s' },
      ].map((t, i) => (
        <path
          key={i}
          d={t.d}
          fill="none"
          stroke="#32CD32"
          strokeWidth="1"
          strokeDasharray="200"
          strokeLinecap="square"
          style={{ animation: `circuit-flow ${t.dur} ${t.delay} linear infinite` }}
        />
      ))}
      {[
        [1360, 280], [1300, 360], [1380, 480], [1310, 440], [1250, 520],
        [1350, 640], [1280, 600], [1370, 760], [1300, 740],
      ].map(([cx, cy], i) => (
        <circle key={i} cx={cx} cy={cy} r="2.5" fill="#32CD32"
          style={{ animation: `node-pulse ${2 + (i % 3)}s ${i * 0.25}s ease-in-out infinite` }}
        />
      ))}

      {/* ── Ocean waves — bottom ── */}
      <g opacity="0.18">
        {/* Wave layer 1 */}
        <g style={{ animation: 'wave-roll 12s linear infinite' }}>
          <path
            d="M-960,820 Q-720,790 -480,820 Q-240,850 0,820 Q240,790 480,820 Q720,850 960,820 Q1200,790 1440,820 Q1680,850 1920,820 Q2160,790 2400,820 L2400,900 L-960,900 Z"
            fill="#32CD32"
          />
        </g>
        {/* Wave layer 2 — offset, slightly different amplitude */}
        <g style={{ animation: 'wave-roll-2 18s linear infinite' }}>
          <path
            d="M-960,840 Q-720,815 -480,840 Q-240,865 0,840 Q240,815 480,840 Q720,865 960,840 Q1200,815 1440,840 Q1680,865 1920,840 L1920,900 L-960,900 Z"
            fill="#32CD32"
            opacity="0.5"
          />
        </g>
        {/* Wave layer 3 — slowest */}
        <g style={{ animation: 'wave-roll 25s linear infinite reverse' }}>
          <path
            d="M-960,855 Q-600,840 -240,855 Q120,870 480,855 Q840,840 1200,855 Q1560,870 1920,855 Q2280,840 2400,855 L2400,900 L-960,900 Z"
            fill="#32CD32"
            opacity="0.3"
          />
        </g>
      </g>

      {/* ── Ship silhouette — bottom center ── */}
      <g transform="translate(680, 800)" opacity="0.07">
        <path d="M80,0 L40,60 L0,55 L-10,80 L170,80 L160,55 L120,60 Z" fill="#32CD32"/>
        <path d="M80,0 L80,-80 L50,-20 Z" fill="#32CD32"/>
        <path d="M80,-10 L80,-80 L110,-20 Z" fill="#32CD32"/>
        <line x1="80" y1="0" x2="80" y2="-85" stroke="#32CD32" strokeWidth="2"/>
        <path d="M-10,80 Q80,95 170,80" fill="none" stroke="#32CD32" strokeWidth="1.5"/>
      </g>

      {/* ── Top corner brackets ── */}
      <polyline points="40,40 20,40 20,70" fill="none" stroke="#32CD32" strokeWidth="1.5" opacity="0.3"/>
      <polyline points="1400,40 1420,40 1420,70" fill="none" stroke="#32CD32" strokeWidth="1.5" opacity="0.3"/>
      <polyline points="40,860 20,860 20,830" fill="none" stroke="#32CD32" strokeWidth="1.5" opacity="0.3"/>
      <polyline points="1400,860 1420,860 1420,830" fill="none" stroke="#32CD32" strokeWidth="1.5" opacity="0.3"/>
    </svg>
  );
}

/* ── Feature icons (inline SVG) ─────────────────────────────────────────── */
function IconShip() {
  return (
    <svg className="landing-feature-icon" viewBox="0 0 40 40" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M20 4 L20 28" stroke="#32CD32" strokeWidth="1.5"/>
      <path d="M20 4 L20 22 L8 14 Z" fill="#32CD32" opacity="0.6"/>
      <path d="M20 8 L20 22 L32 14 Z" fill="#32CD32" opacity="0.4"/>
      <path d="M4 28 L10 26 L20 28 L30 26 L36 28" stroke="#32CD32" strokeWidth="1.8" strokeLinecap="round"/>
      <path d="M8 28 L6 36 L34 36 L32 28" fill="#32CD32" opacity="0.5"/>
      <path d="M4 36 Q20 40 36 36" fill="none" stroke="#32CD32" strokeWidth="1.2"/>
    </svg>
  );
}

function IconLock() {
  return (
    <svg className="landing-feature-icon" viewBox="0 0 40 40" fill="none" xmlns="http://www.w3.org/2000/svg">
      <rect x="8" y="18" width="24" height="18" rx="2" stroke="#32CD32" strokeWidth="1.8"/>
      <path d="M13 18 V13 A7 7 0 0 1 27 13 V18" stroke="#32CD32" strokeWidth="1.8"/>
      <circle cx="20" cy="27" r="3" fill="#32CD32"/>
      <line x1="20" y1="30" x2="20" y2="33" stroke="#32CD32" strokeWidth="1.8" strokeLinecap="round"/>
      {/* Circuit lines radiating */}
      <line x1="4" y1="22" x2="8" y2="22" stroke="#32CD32" strokeWidth="1" opacity="0.4"/>
      <line x1="32" y1="22" x2="36" y2="22" stroke="#32CD32" strokeWidth="1" opacity="0.4"/>
      <line x1="4" y1="26" x2="6" y2="26" stroke="#32CD32" strokeWidth="1" opacity="0.3"/>
      <line x1="34" y1="30" x2="36" y2="30" stroke="#32CD32" strokeWidth="1" opacity="0.3"/>
    </svg>
  );
}

function IconSkull() {
  return (
    <svg className="landing-feature-icon" viewBox="0 0 40 40" fill="none" xmlns="http://www.w3.org/2000/svg">
      <ellipse cx="20" cy="16" rx="13" ry="12" stroke="#32CD32" strokeWidth="1.5"/>
      <path d="M8 22 Q8 30 20 30 Q32 30 32 22" fill="none" stroke="#32CD32" strokeWidth="1.4"/>
      <line x1="15" y1="26" x2="15" y2="30" stroke="#32CD32" strokeWidth="1.2"/>
      <line x1="20" y1="26" x2="20" y2="31" stroke="#32CD32" strokeWidth="1.2"/>
      <line x1="25" y1="26" x2="25" y2="30" stroke="#32CD32" strokeWidth="1.2"/>
      <polygon points="14,12 10,16 14,20 18,16" fill="#32CD32"/>
      <polygon points="26,12 22,16 26,20 30,16" fill="#32CD32"/>
      <polygon points="20,16 17.5,21 22.5,21" fill="#32CD32" opacity="0.7"/>
      <line x1="5" y1="34" x2="35" y2="38" stroke="#32CD32" strokeWidth="2" strokeLinecap="round"/>
      <circle cx="5" cy="34" r="2.5" fill="none" stroke="#32CD32" strokeWidth="1.2"/>
      <circle cx="35" cy="38" r="2.5" fill="none" stroke="#32CD32" strokeWidth="1.2"/>
      <line x1="35" y1="34" x2="5"  y2="38" stroke="#32CD32" strokeWidth="2" strokeLinecap="round"/>
      <circle cx="35" cy="34" r="2.5" fill="none" stroke="#32CD32" strokeWidth="1.2"/>
      <circle cx="5"  cy="38" r="2.5" fill="none" stroke="#32CD32" strokeWidth="1.2"/>
    </svg>
  );
}

/* ── Landing Page ────────────────────────────────────────────────────────── */
export const Landing: React.FC = () => {
  const navigate = useNavigate();
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    return startRain(canvas);
  }, []);

  return (
    <div className="landing">
      <canvas ref={canvasRef} className="landing-canvas" />
      <BgSvg />
      <div className="landing-scanlines" />
      <div className="landing-sweep" />

      <div className="landing-content">
        {/* Nav */}
        <nav className="landing-nav">
          <span className="landing-nav-brand">☠ PARLEY v1.0</span>
          <div className="landing-nav-links">
            <button className="landing-nav-link" onClick={() => navigate('/login')}>Sign In</button>
            <button className="landing-nav-link" onClick={() => navigate('/register')}>Register</button>
          </div>
        </nav>

        {/* Hero */}
        <section className="landing-hero">
          <div className="landing-logo-wrap">
            <img src="/logo.svg" alt="Parley" className="landing-logo" width="480" height="160" />
          </div>

          <div className="landing-tagline">
            The digital seas await<span className="cursor" />
          </div>
          <div className="landing-tagline-sub">
            real-time comms for the digital underground
          </div>

          <div className="landing-ctas">
            <button className="landing-btn landing-btn-primary" onClick={() => navigate('/register')}>
              ☠ Board the Ship
            </button>
            <button className="landing-btn landing-btn-secondary" onClick={() => navigate('/login')}>
              &gt;_ Sign In
            </button>
          </div>

          {/* Divider */}
          <div className="landing-divider">
            <div className="landing-divider-line" />
            <span className="landing-divider-text">// CAPABILITIES //</span>
            <div className="landing-divider-line" />
          </div>

          {/* Features */}
          <div className="landing-features">
            <div className="landing-feature">
              <IconShip />
              <div className="landing-feature-title">Servers &amp; Channels</div>
              <div className="landing-feature-desc">
                Create your fleet. Organize your crew into servers and channels. Voice, text, and everything in between. Your flag, your rules.
              </div>
            </div>
            <div className="landing-feature">
              <IconLock />
              <div className="landing-feature-title">Secure Comms</div>
              <div className="landing-feature-desc">
                TLS on every transmission. Role-based access control. What's spoken in the hold stays in the hold.
              </div>
            </div>
            <div className="landing-feature">
              <IconSkull />
              <div className="landing-feature-title">Direct Messages</div>
              <div className="landing-feature-desc">
                Parley one-on-one. Private channels between any two crew members. Files, images, markdown — the full arsenal.
              </div>
            </div>
          </div>
        </section>

        {/* Footer */}
        <footer className="landing-footer">
          <span>© PARLEY — NEGOTIATE OR DIE</span>
          <span>&#91; ALL TRANSMISSIONS MONITORED BY NO ONE &#93;</span>
        </footer>
      </div>
    </div>
  );
};
