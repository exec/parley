import React from 'react';
import './BadgeList.css';

const CDN = 'https://parley-prod.nyc3.cdn.digitaloceanspaces.com';

const BADGES: { bit: number; label: string; url: string }[] = [
  { bit: 2, label: 'Parley Admin', url: `${CDN}/badges/admin.svg` },
  { bit: 1, label: 'Donor',       url: `${CDN}/badges/donor.svg` },
];

interface BadgeListProps {
  badges: number;
}

const BadgeList: React.FC<BadgeListProps> = ({ badges }) => {
  const active = BADGES.filter(b => (badges & b.bit) !== 0);
  if (active.length === 0) return null;
  return (
    <div className="badge-list">
      {active.map(b => (
        <div key={b.bit} className="badge-item" title={b.label}>
          <img src={b.url} alt={b.label} width={20} height={20} draggable={false} />
        </div>
      ))}
    </div>
  );
};

export default BadgeList;
