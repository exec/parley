import React from 'react';
import { lookup } from '../../activities/registry';
import type { ActivityRecord } from '../../api/activities';

interface Props {
  vc: string;
  activity: ActivityRecord | null;
}

export const ActivitySlot: React.FC<Props> = ({ vc, activity }) => {
  if (!activity) return null;
  const def = lookup(activity.type);
  if (!def) {
    return (
      <div className="activity-slot activity-slot--unknown">
        Activity '{activity.type}' in progress (this client doesn't support it yet).
      </div>
    );
  }
  const Render = def.render;
  return (
    <div className="activity-slot">
      <Render vc={vc} params={activity.params} />
    </div>
  );
};
