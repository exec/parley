import React from 'react';

interface Props {
  children: React.ReactNode;
}

interface State {
  error: Error | null;
}

export class ErrorBoundary extends React.Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    console.error('[ErrorBoundary] Uncaught render error:', error, info.componentStack);
  }

  render() {
    if (this.state.error) {
      return (
        <div style={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100vh',
          background: '#111',
          color: '#eee',
          gap: '12px',
          fontFamily: 'sans-serif',
        }}>
          <h2 style={{ color: '#ff4444', margin: 0 }}>Something went wrong</h2>
          <p style={{ color: '#aaa', margin: 0 }}>Refresh the page to try again.</p>
          <pre style={{
            background: '#1a1a1a',
            padding: '12px 16px',
            borderRadius: '6px',
            color: '#ff6b6b',
            fontSize: '12px',
            maxWidth: '600px',
            overflow: 'auto',
            margin: 0,
          }}>
            {this.state.error.message}
          </pre>
          <button
            onClick={() => window.location.reload()}
            style={{
              marginTop: '8px',
              padding: '8px 20px',
              background: '#32CD32',
              color: '#000',
              border: 'none',
              borderRadius: '4px',
              cursor: 'pointer',
              fontWeight: 600,
            }}
          >
            Reload
          </button>
        </div>
      );
    }

    return this.props.children;
  }
}
