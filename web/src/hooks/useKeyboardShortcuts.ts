import { useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';

interface ShortcutConfig {
  key: string;
  ctrl?: boolean;
  alt?: boolean;
  shift?: boolean;
  handler: () => void;
  description?: string;
}

export function useKeyboardShortcuts(shortcuts: ShortcutConfig[]) {
  const handleKeyPress = useCallback((event: KeyboardEvent) => {
    shortcuts.forEach(shortcut => {
      const { key, ctrl = false, alt = false, shift = false, handler } = shortcut;
      
      if (
        event.key.toLowerCase() === key.toLowerCase() &&
        event.ctrlKey === ctrl &&
        event.altKey === alt &&
        event.shiftKey === shift
      ) {
        event.preventDefault();
        handler();
      }
    });
  }, [shortcuts]);

  useEffect(() => {
    window.addEventListener('keydown', handleKeyPress);
    return () => window.removeEventListener('keydown', handleKeyPress);
  }, [handleKeyPress]);
}

export function useGlobalKeyboardShortcuts() {
  const navigate = useNavigate();

  const shortcuts: ShortcutConfig[] = [
    {
      key: 'd',
      ctrl: true,
      handler: () => navigate('/dashboard'),
      description: 'Go to Dashboard',
    },
    {
      key: 'j',
      ctrl: true,
      handler: () => navigate('/jobs'),
      description: 'Go to Jobs',
    },
    {
      key: 'b',
      ctrl: true,
      handler: () => navigate('/bots'),
      description: 'Go to Bots',
    },
    {
      key: '/',
      ctrl: true,
      handler: () => {
        // Focus search if available
        const searchInput = document.querySelector('input[type="search"]');
        if (searchInput instanceof HTMLInputElement) {
          searchInput.focus();
        }
      },
      description: 'Focus search',
    },
  ];

  useKeyboardShortcuts(shortcuts);
  
  return shortcuts;
}