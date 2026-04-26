import { createRoot } from 'react-dom/client';
import { RingApp } from './RingApp';
import './RingApp.css';

const params = new URLSearchParams(window.location.search);
const props = {
  ringId:            params.get('ring_id') ?? '',
  vc:                params.get('vc') ?? '',
  callerUsername:    params.get('caller_username') ?? '',
  callerDisplayName: params.get('caller_display_name') ?? '',
  callerAvatarUrl:   params.get('caller_avatar_url') ?? '',
  groupName:         params.get('group_name') ?? '',
};

createRoot(document.getElementById('ring-root')!).render(<RingApp {...props} />);
