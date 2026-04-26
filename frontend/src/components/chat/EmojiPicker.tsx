import React, { useState, useEffect, useRef, useCallback, useImperativeHandle } from 'react';
import { createPortal } from 'react-dom';
import data from '@emoji-mart/data';
import { SearchIndex, init } from 'emoji-mart';
init({ data });

const EMOJI_CATEGORIES = [
  {
    name: 'Smileys',
    icon: '😊',
    emojis: ['😀','😃','😄','😁','😆','😅','🤣','😂','🙂','🙃','😉','😊','😇','🥰','😍','🤩','😘','😗','😚','😙','🥲','😋','😛','😜','🤪','😝','🤑','🤗','🤭','🤫','🤔','🤐','🤨','😐','😑','😶','😏','😒','🙄','😬','🤥','😌','😔','😪','🤤','😴','😷','🤒','🤕','🤢','🤮','🤧','🥵','🥶','🥴','😵','💫','🤯','🤠','🥳','🥸','😎','🤓','🧐','😕','😟','🙁','☹️','😮','😯','😲','😳','🥺','😦','😧','😨','😰','😥','😢','😭','😱','😖','😣','😞','😓','😩','😫','🥱','😤','😡','😠','🤬','😈','👿','💀','☠️','💩','🤡','👹','👺','👻','👽','👾','🤖']
  },
  {
    name: 'People',
    icon: '👋',
    emojis: ['👋','🤚','🖐️','✋','🖖','👌','🤌','🤏','✌️','🤞','🤟','🤘','🤙','👈','👉','👆','🖕','👇','☝️','👍','👎','✊','👊','🤛','🤜','👏','🙌','🫶','👐','🤲','🤝','🙏','✍️','💅','🤳','💪','🦾','🦿','🦵','🦶','👂','🦻','👃','👀','👁️','👅','🦷','🫀','🫁','🧠','🦴','💋','💌','💘','💝','💖','💗','💓','💞','💕','💟','❣️','💔','❤️','🧡','💛','💚','💙','💜','🤎','🖤','🤍']
  },
  {
    name: 'Animals',
    icon: '🐶',
    emojis: ['🐶','🐱','🐭','🐹','🐰','🦊','🐻','🐼','🐨','🐯','🦁','🐮','🐷','🐸','🐵','🙈','🙉','🙊','🐔','🐧','🐦','🐤','🦆','🦅','🦉','🦇','🐺','🐗','🐴','🦄','🐝','🐛','🦋','🐌','🐞','🐜','🦟','🦗','🕷️','🦂','🐢','🐍','🦎','🦖','🦕','🐙','🦑','🦐','🦞','🦀','🐡','🐠','🐟','🐬','🐳','🐋','🦈','🐊','🐅','🐆','🦓','🦍','🦧','🐘','🦛','🦏','🐪','🐫','🦒','🦘','🐃','🐂','🐄','🐎','🐖','🐏','🐑','🦙','🐐','🦌','🐕','🐩','🦮','🐕‍🦺','🐈','🐈‍⬛','🪶','🐓','🦃','🦤','🦚','🦜','🦢','🦩','🕊️','🐇','🦝','🦨','🦡','🦦','🦥','🐁','🐀','🐿️','🦔']
  },
  {
    name: 'Food',
    icon: '🍕',
    emojis: ['🍎','🍐','🍊','🍋','🍌','🍉','🍇','🍓','🫐','🍈','🍒','🍑','🥭','🍍','🥥','🥝','🍅','🍆','🥑','🥦','🥬','🥒','🌶️','🫑','🧄','🧅','🥔','🍠','🥐','🥯','🍞','🥖','🥨','🧀','🥚','🍳','🧈','🥞','🧇','🥓','🥩','🍗','🍖','🌭','🍔','🍟','🍕','🫓','🥪','🥙','🧆','🌮','🌯','🫔','🥗','🥘','🫕','🥫','🍱','🍘','🍙','🍚','🍛','🍜','🍝','🍠','🍢','🍣','🍤','🍥','🥮','🍡','🥟','🥠','🥡','🍦','🍧','🍨','🍩','🍪','🎂','🍰','🧁','🥧','🍫','🍬','🍭','🍮','🍯','🍼','🥛','☕','🫖','🍵','🧃','🥤','🧋','🍶','🍺','🍻','🥂','🍷','🥃','🍸','🍹','🧉','🍾']
  },
  {
    name: 'Activities',
    icon: '⚽',
    emojis: ['⚽','🏀','🏈','⚾','🥎','🎾','🏐','🏉','🥏','🎱','🏓','🏸','🏒','🏑','🥍','🏏','🪃','🥅','⛳','🪁','🏹','🎣','🤿','🥊','🥋','🎽','🛹','🛼','🛷','⛸️','🥌','🎿','⛷️','🏂','🪂','🏋️','🤼','🤸','⛹️','🤺','🏇','🧘','🏄','🏊','🤽','🚣','🧗','🚵','🚴','🏆','🥇','🥈','🥉','🏅','🎖️','🏵️','🎗️','🎫','🎟️','🎪','🤹','🎭','🩰','🎨','🎬','🎤','🎧','🎼','🎵','🎶','🥁','🪘','🎷','🎺','🪗','🎸','🪕','🎻','🎲','♟️','🎯','🎳','🎮','🎰','🧩']
  },
  {
    name: 'Travel',
    icon: '✈️',
    emojis: ['🚗','🚕','🚙','🚌','🚎','🏎️','🚓','🚑','🚒','🚐','🛻','🚚','🚛','🚜','🦯','🦽','🦼','🛴','🚲','🛵','🏍️','🛺','🚨','🚥','🚦','🛑','🚧','⚓','🛟','⛵','🚤','🛥️','🛳️','⛴️','🚢','✈️','🛩️','🛫','🛬','🪂','💺','🚁','🚟','🚠','🚡','🛰️','🚀','🛸','🎆','🎇','🗺️','🗾','🏔️','⛰️','🌋','🗻','🏕️','🏖️','🏜️','🏝️','🏞️','🏟️','🏛️','🏗️','🧱','🪨','🪵','🛖','🏘️','🏚️','🏠','🏡','🏢','🏣','🏤','🏥','🏦','🏨','🏩','🏪','🏫','🏬','🏭','🏯','🏰','💒','🗼','🗽','⛪','🕌','🛕','🕍','⛩️','🕋']
  },
  {
    name: 'Objects',
    icon: '💡',
    emojis: ['⌚','📱','📲','💻','⌨️','🖥️','🖨️','🖱️','🖲️','🕹️','🗜️','💽','💾','💿','📀','📼','📷','📸','📹','🎥','📽️','🎞️','📞','☎️','📟','📠','📺','📻','🧭','⏱️','⏲️','⏰','🕰️','⌛','⏳','📡','🔋','🪫','🔌','💡','🔦','🕯️','🪔','🧯','🛢️','💰','💴','💵','💶','💷','💸','💳','🪙','💹','✉️','📧','📨','📩','📤','📥','📦','📫','📪','📬','📭','📮','🗳️','✏️','✒️','🖋️','🖊️','📝','📁','📂','🗂️','📅','📆','🗒️','🗓️','📇','📈','📉','📊','📋','📌','📍','🗺️','📎','🖇️','📏','📐','✂️','🗃️','🗄️','🗑️','🔒','🔓','🔏','🔐','🔑','🗝️','🔨','🪓','⛏️','⚒️','🛠️','🗡️','⚔️','🛡️','🪚','🔧','🪛','🔩','⚙️','🗜️','🔗','⛓️','🧰','🧲','🪜']
  },
  {
    name: 'Symbols',
    icon: '❤️',
    emojis: ['❤️','🧡','💛','💚','💙','💜','🖤','🤍','🤎','💔','❤️‍🔥','❤️‍🩹','❣️','💕','💞','💓','💗','💖','💘','💝','💟','☮️','✝️','☪️','🕉️','☸️','✡️','🔯','🕎','☯️','☦️','🛐','⛎','♈','♉','♊','♋','♌','♍','♎','♏','♐','♑','♒','♓','🆔','⚛️','🉑','☢️','☣️','📴','📳','🈶','🈚','🈸','🈺','🈷️','✴️','🆚','💮','🉐','㊙️','㊗️','🈴','🈵','🈹','🈲','🅰️','🅱️','🆎','🆑','🅾️','🆘','❌','⭕','🛑','⛔','📛','🚫','💯','💢','♨️','🚷','🚯','🚳','🚱','🔞','📵','🔕','🔇','🔈','🔉','🔊','📢','📣','📯','🔔','🔕','🎵','🎶','⚠️','🚸','🔱','⚜️','🔰','♻️','✅','🈯','💹','❇️','✳️','❎','🌐','💠','Ⓜ️','🌀','💤','🏧','🚾','♿','🅿️','🛗','🈳','🈹','🚺','🚹','🚼','⚧️','🚻','🚮','🎦','📶','🈁','🔣','ℹ️','🔤','🔡','🔠','🆖','🆗','🆙','🆒','🆕','🆓','0️⃣','1️⃣','2️⃣','3️⃣','4️⃣','5️⃣','6️⃣','7️⃣','8️⃣','9️⃣','🔟','🔢','#️⃣','*️⃣','▶️','⏸️','⏹️','⏺️','⏭️','⏮️','⏩','⏪','⏫','⏬','◀️','🔼','🔽','➡️','⬅️','⬆️','⬇️','↗️','↘️','↙️','↖️','↕️','↔️','↪️','↩️','⤴️','⤵️','🔀','🔁','🔂','🔄','🔃','🎵','🎶','➕','➖','➗','✖️','💲','💱','™️','©️','®️','〰️','➰','➿','🔚','🔙','🔛','🔜','🔝','✔️','☑️','🔘','🔲','🔳','▪️','▫️','◾','◽','◼️','◻️','🟥','🟧','🟨','🟩','🟦','🟪','⬛','⬜','🔶','🔷','🔸','🔹','🔺','🔻','💠','🔘','🔗']
  },
];

interface EmojiPickerProps {
  onSelect: (emoji: string) => void;
  onClose: () => void;
}

// Picker dimensions — must match the .emoji-picker-full CSS rules below.
const PICKER_WIDTH = 340;
const PICKER_MAX_HEIGHT = 400;
const VIEWPORT_MARGIN = 8;
// Extra gap between trigger and picker, matches GifPicker spacing
const TRIGGER_GAP = 8;

export const EmojiPicker = React.forwardRef<HTMLDivElement, EmojiPickerProps>(({ onSelect }, ref) => {
  const [activeCategory, setActiveCategory] = useState(0);
  const [search, setSearch] = useState('');
  const [searchResults, setSearchResults] = useState<string[]>([]);

  // Anchor pattern: an invisible inline span lives where this component is rendered;
  // we measure its parent element's bounding rect to position the (portaled) picker
  // adjacent to its trigger area, regardless of where the trigger lives in the DOM.
  const anchorRef = useRef<HTMLSpanElement>(null);
  const pickerRef = useRef<HTMLDivElement>(null);
  // Forward the picker's DOM node out for callers that need it (outside-click checks).
  useImperativeHandle(ref, () => pickerRef.current as HTMLDivElement);

  const [pos, setPos] = useState<{ top: number; left: number } | null>(null);

  const recomputePos = useCallback(() => {
    const anchor = anchorRef.current?.parentElement;
    if (!anchor) return;
    const rect = anchor.getBoundingClientRect();
    const vw = window.innerWidth;
    const vh = window.innerHeight;

    // Prefer above the anchor; fall back to below if there isn't room above
    // and there's more room below.
    const spaceAbove = rect.top;
    const spaceBelow = vh - rect.bottom;
    const placeAbove =
      spaceAbove >= PICKER_MAX_HEIGHT + TRIGGER_GAP + VIEWPORT_MARGIN ||
      spaceAbove >= spaceBelow;

    let top: number;
    if (placeAbove) {
      top = Math.max(VIEWPORT_MARGIN, rect.top - TRIGGER_GAP - PICKER_MAX_HEIGHT);
    } else {
      top = Math.min(
        rect.bottom + TRIGGER_GAP,
        vh - VIEWPORT_MARGIN - PICKER_MAX_HEIGHT,
      );
      // If there's not enough room below either, clamp so the picker stays in view
      // even if it overlaps the trigger slightly.
      if (top < VIEWPORT_MARGIN) top = VIEWPORT_MARGIN;
    }

    // Horizontally align with the anchor's right edge (the prior CSS used `right: 0`),
    // then clamp to the viewport.
    let left = rect.right - PICKER_WIDTH;
    if (left + PICKER_WIDTH > vw - VIEWPORT_MARGIN) {
      left = vw - VIEWPORT_MARGIN - PICKER_WIDTH;
    }
    if (left < VIEWPORT_MARGIN) left = VIEWPORT_MARGIN;

    setPos({ top, left });
  }, []);

  // Recompute on mount + window resize/scroll. ResizeObserver handles the
  // common "user resized split" case.
  useEffect(() => {
    recomputePos();
    const onResize = () => recomputePos();
    window.addEventListener('resize', onResize);
    window.addEventListener('scroll', onResize, true);
    const anchor = anchorRef.current?.parentElement;
    let ro: ResizeObserver | null = null;
    if (anchor && typeof ResizeObserver !== 'undefined') {
      ro = new ResizeObserver(recomputePos);
      ro.observe(anchor);
    }
    return () => {
      window.removeEventListener('resize', onResize);
      window.removeEventListener('scroll', onResize, true);
      ro?.disconnect();
    };
  }, [recomputePos]);

  useEffect(() => {
    if (!search.trim()) {
      setSearchResults([]);
      return;
    }
    (SearchIndex as any).search(search).then((results: any[]) => {
      setSearchResults(results.map((r: any) => r.skins[0].native));
    });
  }, [search]);

  const displayEmojis: string[] = search.trim() ? searchResults : EMOJI_CATEGORIES[activeCategory].emojis;

  const portal = pos
    ? createPortal(
        <div
          ref={pickerRef}
          className="emoji-picker-full"
          style={{
            position: 'fixed',
            top: pos.top,
            left: pos.left,
            right: 'auto',
            bottom: 'auto',
          }}
          onClick={e => e.stopPropagation()}
        >
          <div className="emoji-picker-search">
            <input
              autoFocus
              type="text"
              placeholder="Search emoji..."
              value={search}
              onChange={e => setSearch(e.target.value)}
              className="emoji-search-input"
            />
          </div>
          {!search && (
            <div className="emoji-category-tabs">
              {EMOJI_CATEGORIES.map((cat, i) => (
                <button
                  key={cat.name}
                  className={`emoji-cat-btn${activeCategory === i ? ' active' : ''}`}
                  onClick={() => setActiveCategory(i)}
                  title={cat.name}
                >
                  {cat.icon}
                </button>
              ))}
            </div>
          )}
          <div className="emoji-grid">
            {displayEmojis.map((emoji, i) => (
              <button key={i} className="emoji-grid-btn" onClick={() => onSelect(emoji)}>
                {emoji}
              </button>
            ))}
          </div>
        </div>,
        document.body,
      )
    : null;

  return (
    <>
      <span ref={anchorRef} style={{ display: 'none' }} aria-hidden="true" />
      {portal}
    </>
  );
});
