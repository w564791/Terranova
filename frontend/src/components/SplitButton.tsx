import React, { useState, useRef, useEffect } from 'react';
import styles from './SplitButton.module.css';

interface MenuItem {
  label: string;
  onClick: () => void;
  icon?: string;
}

interface SplitButtonProps {
  mainLabel: string;
  mainOnClick: () => void;
  menuItems: MenuItem[];
  disabled?: boolean;
  className?: string;
}

const SplitButton: React.FC<SplitButtonProps> = ({
  mainLabel,
  mainOnClick,
  menuItems,
  disabled = false,
  className = ''
}) => {
  const [isOpen, setIsOpen] = useState(false);
  const [dropdownPosition, setDropdownPosition] = useState<'bottom' | 'top'>('bottom');
  const dropdownRef = useRef<HTMLDivElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setIsOpen(false);
      }
    };

    if (isOpen) {
      document.addEventListener('mousedown', handleClickOutside);
      
      // Check position after menu is rendered
      if (menuRef.current && dropdownRef.current) {
        const buttonRect = dropdownRef.current.getBoundingClientRect();
        const menuHeight = menuRef.current.offsetHeight;
        const viewportHeight = window.innerHeight;
        const spaceBelow = viewportHeight - buttonRect.bottom;
        const spaceAbove = buttonRect.top;
        
        console.log('Button bottom:', buttonRect.bottom);
        console.log('Menu height:', menuHeight);
        console.log('Viewport height:', viewportHeight);
        console.log('Space below:', spaceBelow);
        console.log('Space above:', spaceAbove);
        
        // If not enough space below but enough space above, show menu above
        if (spaceBelow < menuHeight + 10 && spaceAbove > menuHeight + 10) {
          console.log('Setting position to TOP');
          setDropdownPosition('top');
        } else {
          console.log('Setting position to BOTTOM');
          setDropdownPosition('bottom');
        }
      }
    }

    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, [isOpen]);

  const handleMainClick = () => {
    if (!disabled) {
      mainOnClick();
    }
  };

  const handleDropdownClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!disabled) {
      setIsOpen(!isOpen);
    }
  };

  const handleMenuItemClick = (onClick: () => void) => {
    onClick();
    setIsOpen(false);
  };

  return (
    <div className={`${styles.splitButton} ${className}`} ref={dropdownRef}>
      <button
        className={`${styles.mainButton} ${disabled ? styles.disabled : ''}`}
        onClick={handleMainClick}
        disabled={disabled}
      >
        {mainLabel}
      </button>
      <button
        className={`${styles.dropdownButton} ${disabled ? styles.disabled : ''}`}
        onClick={handleDropdownClick}
        disabled={disabled}
      >
        <svg
          width="12"
          height="12"
          viewBox="0 0 12 12"
          fill="none"
          xmlns="http://www.w3.org/2000/svg"
          className={styles.dropdownIcon}
        >
          <path
            d="M2.5 4.5L6 8L9.5 4.5"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      </button>
      
      {isOpen && (
        <div 
          ref={menuRef}
          className={`${styles.dropdownMenu} ${dropdownPosition === 'top' ? styles.dropdownMenuTop : ''}`}
        >
          {menuItems.map((item, index) => (
            <button
              key={index}
              className={styles.menuItem}
              onClick={() => handleMenuItemClick(item.onClick)}
            >
              {item.icon && <span className={styles.menuIcon}>{item.icon}</span>}
              <span>{item.label}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
};

export default SplitButton;
