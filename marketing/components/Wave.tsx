import { useId } from "react";

/** Courbe de solde : teal pour le passé, lilas pour le futur (comme dans l'app). */
export function Wave({ split = 0.62, marker = true }: { split?: number; marker?: boolean }) {
  const id = useId();
  return (
    <svg viewBox="0 0 300 96" preserveAspectRatio="none" aria-hidden="true">
      <defs>
        <linearGradient id={`${id}f`} x1="0" x2="0" y1="0" y2="1">
          <stop offset="0" stopColor="var(--accent)" stopOpacity="0.35" />
          <stop offset="1" stopColor="var(--accent)" stopOpacity="0" />
        </linearGradient>
        <linearGradient id={`${id}l`} x1="0" x2="1" y1="0" y2="0">
          <stop offset="0" stopColor="var(--accent)" />
          <stop offset={split} stopColor="var(--accent)" />
          <stop offset={split} stopColor="var(--lilac)" />
          <stop offset="1" stopColor="var(--lilac)" />
        </linearGradient>
      </defs>
      <path d="M0 62 C30 50 50 70 85 56 S140 30 175 44 S235 70 300 34 L300 96 L0 96 Z" fill={`url(#${id}f)`} />
      <path d="M0 62 C30 50 50 70 85 56 S140 30 175 44 S235 70 300 34" fill="none" stroke={`url(#${id}l)`} strokeWidth="4" strokeLinecap="round" />
      {marker && <circle cx="185" cy="43" r="6" fill="var(--surface)" stroke="var(--accent)" strokeWidth="3" />}
    </svg>
  );
}
