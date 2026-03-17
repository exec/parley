// frontend/src/pages/BotInvitePage.tsx
import React from 'react';
import { useParams } from 'react-router-dom';
import { ThemeProvider } from '../context/ThemeContext';
import { BotInviteEmbed } from '../components/BotInviteEmbed';

export const BotInvitePage: React.FC = () => {
  const { token } = useParams<{ token: string }>();
  if (!token) return null;
  return (
    <ThemeProvider>
      <div style={{ minHeight: '100vh', background: 'var(--parley-channel-bg,#000)', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20 }}>
        <BotInviteEmbed token={token} />
      </div>
    </ThemeProvider>
  );
};
