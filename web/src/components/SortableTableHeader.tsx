import React from 'react';
import { TableCell, TableCellProps } from '@mui/material';
import { styled } from '@mui/material/styles';

export type SortDirection = 'asc' | 'desc';

export interface SortState {
  key: string;
  direction: SortDirection;
}

interface SortableTableHeaderProps extends Omit<TableCellProps, 'onClick'> {
  label: string;
  sortKey: string;
  currentSort: SortState | null;
  onSort: (sortKey: string) => void;
}

const StyledTableCell = styled(TableCell)(({ theme }) => ({
  cursor: 'pointer',
  userSelect: 'none',
  '&:hover': {
    backgroundColor: theme.palette.action.hover,
  },
  '& .sort-indicator': {
    marginLeft: theme.spacing(0.5),
    fontSize: '0.875rem',
    verticalAlign: 'middle',
    color: theme.palette.text.secondary,
    '&.active': {
      color: theme.palette.primary.main,
    },
  },
}));

export const SortableTableHeader: React.FC<SortableTableHeaderProps> = ({
  label,
  sortKey,
  currentSort,
  onSort,
  ...tableCellProps
}) => {
  const isActive = currentSort?.key === sortKey;
  const sortDirection = isActive ? currentSort.direction : null;

  const handleClick = () => {
    onSort(sortKey);
  };

  const getSortIndicator = () => {
    if (!isActive) {
      return null;
    }
    return (
      <span className="sort-indicator active">
        {sortDirection === 'asc' ? '▲' : '▼'}
      </span>
    );
  };

  return (
    <StyledTableCell onClick={handleClick} {...tableCellProps}>
      {label}
      {getSortIndicator()}
    </StyledTableCell>
  );
};

// Helper hook for managing sort state
export const useSort = (initialSort?: SortState) => {
  const [sortState, setSortState] = React.useState<SortState | null>(
    initialSort || null
  );

  const handleSort = React.useCallback((key: string) => {
    setSortState((prevSort) => {
      if (prevSort?.key === key) {
        // Toggle direction if same key
        return {
          key,
          direction: prevSort.direction === 'asc' ? 'desc' : 'asc',
        };
      } else {
        // New key, default to ascending
        return {
          key,
          direction: 'asc',
        };
      }
    });
  }, []);

  const sortData = React.useCallback(
    <T extends Record<string, any>>(data: T[]): T[] => {
      if (!sortState) {
        return data;
      }

      return [...data].sort((a, b) => {
        const aValue = a[sortState.key];
        const bValue = b[sortState.key];

        if (aValue === null || aValue === undefined) return 1;
        if (bValue === null || bValue === undefined) return -1;

        let comparison = 0;
        if (typeof aValue === 'string' && typeof bValue === 'string') {
          comparison = aValue.localeCompare(bValue);
        } else if (typeof aValue === 'number' && typeof bValue === 'number') {
          comparison = aValue - bValue;
        } else {
          comparison = String(aValue).localeCompare(String(bValue));
        }

        return sortState.direction === 'asc' ? comparison : -comparison;
      });
    },
    [sortState]
  );

  return {
    sortState,
    handleSort,
    sortData,
  };
};