import React, { useState, useEffect, useRef } from 'react';
import { Mic, Volume2, Video, Settings } from 'lucide-react';
import { loadVoiceSettings, VoiceSettings, persistVoiceSettings } from '../../hooks/useVoiceConnection';
import './VoiceSettings.css';

export const VoiceSettingsTab: React.FC = () => {
  const [settings, setSettings] = useState<VoiceSettings>(loadVoiceSettings);
  const [inputDevices, setInputDevices] = useState<MediaDeviceInfo[]>([]);
  const [outputDevices, setOutputDevices] = useState<MediaDeviceInfo[]>([]);
  const [videoDevices, setVideoDevices] = useState<MediaDeviceInfo[]>([]);
  const [bindingPtt, setBindingPtt] = useState(false);
  const [testRecording, setTestRecording] = useState<'idle' | 'recording' | 'playing'>('idle');
  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const chunksRef = useRef<Blob[]>([]);

  useEffect(() => {
    navigator.mediaDevices.enumerateDevices().then(devices => {
      setInputDevices(devices.filter(d => d.kind === 'audioinput'));
      setOutputDevices(devices.filter(d => d.kind === 'audiooutput'));
      setVideoDevices(devices.filter(d => d.kind === 'videoinput'));
    }).catch(() => {});
  }, []);

  const update = (patch: Partial<VoiceSettings>) => {
    const next = { ...settings, ...patch };
    setSettings(next);
    persistVoiceSettings(patch);
    if (patch.speakerDeviceId) {
      document.querySelectorAll('audio').forEach(el => {
        if ('setSinkId' in el) (el as any).setSinkId(patch.speakerDeviceId).catch(() => {});
      });
    }
  };

  const startPttBind = () => {
    setBindingPtt(true);
    const handler = (e: KeyboardEvent) => {
      e.preventDefault();
      update({ pttKey: e.code });
      setBindingPtt(false);
      window.removeEventListener('keydown', handler, true);
    };
    window.addEventListener('keydown', handler, true);
  };

  const testMic = async () => {
    if (testRecording !== 'idle') return;
    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: { deviceId: settings.micDeviceId ? { exact: settings.micDeviceId } : undefined },
      });
      const recorder = new MediaRecorder(stream);
      chunksRef.current = [];
      recorder.ondataavailable = e => chunksRef.current.push(e.data);
      recorder.onstop = () => {
        stream.getTracks().forEach(t => t.stop());
        const blob = new Blob(chunksRef.current, { type: 'audio/webm' });
        const url = URL.createObjectURL(blob);
        const audio = new Audio(url);
        setTestRecording('playing');
        audio.onended = () => { setTestRecording('idle'); URL.revokeObjectURL(url); };
        audio.play();
      };
      mediaRecorderRef.current = recorder;
      recorder.start();
      setTestRecording('recording');
      setTimeout(() => { recorder.stop(); }, 3000);
    } catch {
      setTestRecording('idle');
    }
  };

  const pttLabel = settings.pttKey.replace('Key', '').replace('Digit', '').replace('Space', 'SPACE');

  return (
    <div className="vs-container">
      <h2 className="vs-heading">Voice &amp; Video</h2>

      {/* Input Device */}
      <div className="vs-section">
        <label className="vs-label"><Mic size={13} /> Input Device</label>
        <select
          className="vs-select"
          value={settings.micDeviceId ?? ''}
          onChange={e => update({ micDeviceId: e.target.value || undefined })}
        >
          <option value="">Default</option>
          {inputDevices.map(d => <option key={d.deviceId} value={d.deviceId}>{d.label || `Microphone ${d.deviceId.slice(0, 8)}`}</option>)}
        </select>
        <button className="vs-test-btn" onClick={testMic} disabled={testRecording !== 'idle'}>
          {testRecording === 'recording' ? 'Recording… (3s)' : testRecording === 'playing' ? 'Playing back…' : 'Test Microphone'}
        </button>
      </div>

      {/* Output Device */}
      <div className="vs-section">
        <label className="vs-label"><Volume2 size={13} /> Output Device</label>
        <select
          className="vs-select"
          value={settings.speakerDeviceId ?? ''}
          onChange={e => update({ speakerDeviceId: e.target.value || undefined })}
        >
          <option value="">Default</option>
          {outputDevices.map(d => <option key={d.deviceId} value={d.deviceId}>{d.label || `Speaker ${d.deviceId.slice(0, 8)}`}</option>)}
        </select>
      </div>

      {/* Camera */}
      <div className="vs-section">
        <label className="vs-label"><Video size={13} /> Camera</label>
        <select
          className="vs-select"
          value={settings.cameraDeviceId ?? ''}
          onChange={e => update({ cameraDeviceId: e.target.value || undefined })}
        >
          <option value="">Default</option>
          {videoDevices.map(d => <option key={d.deviceId} value={d.deviceId}>{d.label || `Camera ${d.deviceId.slice(0, 8)}`}</option>)}
        </select>
      </div>

      {/* Noise Suppression */}
      <div className="vs-section vs-section--row">
        <div className="vs-section-info">
          <label className="vs-label">Noise Suppression</label>
          <span className="vs-hint">Reduces background noise using browser audio processing</span>
        </div>
        <button
          className={`vs-toggle ${settings.noiseSuppression ? 'vs-toggle--on' : ''}`}
          onClick={() => update({ noiseSuppression: !settings.noiseSuppression })}
        >
          {settings.noiseSuppression ? 'On' : 'Off'}
        </button>
      </div>

      {/* Voice Mode */}
      <div className="vs-section">
        <label className="vs-label"><Settings size={13} /> Voice Mode</label>
        <div className="vs-radio-group">
          {(['vad', 'ptt', 'always'] as const).map(mode => (
            <label key={mode} className={`vs-radio-option ${settings.vadMode === mode ? 'active' : ''}`}>
              <input
                type="radio"
                name="vadMode"
                value={mode}
                checked={settings.vadMode === mode}
                onChange={() => update({ vadMode: mode })}
              />
              <span className="vs-radio-label">
                {mode === 'vad' ? 'Voice Activity' : mode === 'ptt' ? 'Push to Talk' : 'Always On'}
              </span>
              <span className="vs-radio-desc">
                {mode === 'vad' ? 'Auto-detects speech' : mode === 'ptt' ? 'Hold key to transmit' : 'Microphone always open'}
              </span>
            </label>
          ))}
        </div>

        {settings.vadMode === 'ptt' && (
          <div className="vs-ptt-bind">
            <span className="vs-hint">Push to Talk key:</span>
            <button
              className={`vs-keybind-btn ${bindingPtt ? 'listening' : ''}`}
              onClick={startPttBind}
            >
              {bindingPtt ? 'Press any key…' : <kbd>{pttLabel}</kbd>}
            </button>
          </div>
        )}
      </div>
    </div>
  );
};
