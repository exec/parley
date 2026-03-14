import React from 'react';

interface TypingUser {
  userId: string;
  username: string;
}

interface TypingIndicatorProps {
  typingUsers: TypingUser[];
}

function formatTypingText(users: TypingUser[]): string {
  if (users.length === 1) return `${users[0].username} is typing`;
  if (users.length === 2) return `${users[0].username} and ${users[1].username} are typing`;
  if (users.length === 3) return `${users[0].username}, ${users[1].username}, and ${users[2].username} are typing`;
  return `${users[0].username}, ${users[1].username}, and ${users.length - 2} others are typing`;
}

export const TypingIndicator: React.FC<TypingIndicatorProps> = ({ typingUsers }) => {
  if (typingUsers.length === 0) return null;

  return (
    <div className="typing-indicator">
      <span className="typing-dots">
        <span className="typing-dot" />
        <span className="typing-dot" />
        <span className="typing-dot" />
      </span>
      <span className="typing-text">{formatTypingText(typingUsers)}</span>
    </div>
  );
};
