import React from 'react';
import { Box, Typography, Button, SvgIcon } from '@mui/material';
import { Add as AddIcon } from '@mui/icons-material';

interface EmptyStateProps {
  icon?: React.ReactNode;
  title: string;
  description?: string;
  actionLabel?: string;
  onAction?: () => void;
}

export const EmptyState: React.FC<EmptyStateProps> = ({
  icon,
  title,
  description,
  actionLabel,
  onAction,
}) => {
  return (
    <Box
      display="flex"
      flexDirection="column"
      alignItems="center"
      justifyContent="center"
      textAlign="center"
      py={8}
      px={3}
    >
      {icon && (
        <Box
          sx={{
            mb: 3,
            color: 'text.secondary',
            opacity: 0.5,
            '& svg': {
              fontSize: 64,
            },
          }}
        >
          {icon}
        </Box>
      )}
      <Typography variant="h6" color="text.secondary" gutterBottom>
        {title}
      </Typography>
      {description && (
        <Typography variant="body2" color="text.secondary" sx={{ mb: 3, maxWidth: 400 }}>
          {description}
        </Typography>
      )}
      {actionLabel && onAction && (
        <Button
          variant="outlined"
          startIcon={<AddIcon />}
          onClick={onAction}
          sx={{
            borderRadius: 2,
            textTransform: 'none',
          }}
        >
          {actionLabel}
        </Button>
      )}
    </Box>
  );
};