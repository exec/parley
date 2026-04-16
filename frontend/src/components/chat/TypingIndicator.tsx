import React from 'react';
import { ServerMember } from '../../api/types';
import { Avatar } from '../ui/Avatar';

interface TypingUser {
  userId: string;
  username: string;
}

interface TypingIndicatorProps {
  typingUsers: TypingUser[];
  members?: ServerMember[];
}

function formatTypingText(users: TypingUser[]): string {
  if (users.length === 1) return `${users[0].username} is typing`;
  if (users.length === 2) return `${users[0].username} and ${users[1].username} are typing`;
  if (users.length === 3) return `${users[0].username}, ${users[1].username}, and ${users[2].username} are typing`;
  return `${users[0].username}, ${users[1].username}, and ${users.length - 2} others are typing`;
}

export const TypingIndicator: React.FC<TypingIndicatorProps> = ({ typingUsers, members }) => {
  if (typingUsers.length === 0) return null;

  const showAvatars = typingUsers.length <= 2;

  return (
    <div className="typing-indicator">
      {showAvatars && (
        <span className="typing-avatars">
          {typingUsers.map(tu => {
            const m = members?.find(mm => mm.user_id === tu.userId);
            return (
              <Avatar
                key={tu.userId}
                size="sm"
                src={m?.avatar_url}
                alt={tu.username}
                fallback={m?.display_name || tu.username}
              />
            );
          })}
        </span>
      )}
      <span className="typing-dots">
        <span className="typing-dot" />
        <span className="typing-dot" />
        <span className="typing-dot" />
      </span>
      <span className="typing-text">{formatTypingText(typingUsers)}</span>
    </div>
  );
};
