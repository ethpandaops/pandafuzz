/**
 * Date formatting utilities for PandaFuzz web application
 * Handles date display formatting with year correction to prevent future dates
 */

export interface FormatDateOptions {
  includeTime?: boolean;
  includeSeconds?: boolean;
  timeZone?: string;
  locale?: string;
}

/**
 * Corrects date if it appears to be in the future due to parsing issues
 * This can happen when dates are parsed incorrectly causing years to appear in the future
 */
const correctFutureDate = (date: Date): Date => {
  const now = new Date();
  const currentYear = now.getFullYear();
  const dateYear = date.getFullYear();
  
  // If the date year is significantly in the future (more than 1 year from now),
  // it's likely a parsing error and we should correct it
  if (dateYear > currentYear + 1) {
    const correctedDate = new Date(date);
    // Set the year to current year - this is a reasonable assumption for most cases
    correctedDate.setFullYear(currentYear);
    return correctedDate;
  }
  
  return date;
};

/**
 * Parse and validate a date string or Date object
 */
const parseDate = (dateInput: string | Date): Date => {
  let date: Date;
  
  if (typeof dateInput === 'string') {
    date = new Date(dateInput);
  } else {
    date = dateInput;
  }
  
  // Check if date is valid
  if (isNaN(date.getTime())) {
    throw new Error('Invalid date provided');
  }
  
  return correctFutureDate(date);
};

/**
 * Format a date string or Date object for display with locale support
 * 
 * @param dateInput - Date string or Date object to format
 * @param options - Formatting options
 * @returns Formatted date string
 */
export const formatDate = (
  dateInput: string | Date, 
  options: FormatDateOptions = {}
): string => {
  const {
    includeTime = true,
    includeSeconds = false,
    timeZone,
    locale = 'en-US'
  } = options;
  
  try {
    const date = parseDate(dateInput);
    
    const formatOptions: Intl.DateTimeFormatOptions = {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      ...(timeZone && { timeZone }),
    };
    
    if (includeTime) {
      formatOptions.hour = '2-digit';
      formatOptions.minute = '2-digit';
      if (includeSeconds) {
        formatOptions.second = '2-digit';
      }
    }
    
    return date.toLocaleDateString(locale, formatOptions);
  } catch (error) {
    console.warn('Date formatting error:', error);
    return 'Invalid Date';
  }
};

/**
 * Format a date for time-only display (e.g., logs)
 * 
 * @param dateInput - Date string or Date object to format
 * @param includeSeconds - Whether to include seconds in the output
 * @returns Formatted time string
 */
export const formatTime = (
  dateInput: string | Date,
  includeSeconds: boolean = true
): string => {
  try {
    const date = parseDate(dateInput);
    
    const formatOptions: Intl.DateTimeFormatOptions = {
      hour: '2-digit',
      minute: '2-digit',
      ...(includeSeconds && { second: '2-digit' }),
    };
    
    return date.toLocaleTimeString('en-US', formatOptions);
  } catch (error) {
    console.warn('Time formatting error:', error);
    return 'Invalid Time';
  }
};

/**
 * Format a date and time with full locale string (legacy compatibility)
 * 
 * @param dateInput - Date string or Date object to format
 * @returns Formatted date and time string
 */
export const formatDateTime = (dateInput: string | Date): string => {
  try {
    const date = parseDate(dateInput);
    return date.toLocaleString('en-US');
  } catch (error) {
    console.warn('DateTime formatting error:', error);
    return 'Invalid Date';
  }
};

/**
 * Format a date for compact display (date only, no time)
 * 
 * @param dateInput - Date string or Date object to format
 * @returns Formatted date string without time
 */
export const formatDateOnly = (dateInput: string | Date): string => {
  return formatDate(dateInput, { includeTime: false });
};

/**
 * Calculate and format duration between two dates
 * 
 * @param startDate - Start date string or Date object
 * @param endDate - End date string or Date object (defaults to now)
 * @returns Formatted duration string
 */
export const formatDuration = (
  startDate: string | Date,
  endDate?: string | Date
): string => {
  try {
    const start = parseDate(startDate);
    const end = endDate ? parseDate(endDate) : new Date();
    
    const diffMs = end.getTime() - start.getTime();
    
    if (diffMs < 0) {
      return '0h 0m';
    }
    
    const hours = Math.floor(diffMs / 3600000);
    const minutes = Math.floor((diffMs % 3600000) / 60000);
    const seconds = Math.floor((diffMs % 60000) / 1000);
    
    if (hours > 0) {
      return `${hours}h ${minutes}m`;
    } else if (minutes > 0) {
      return `${minutes}m ${seconds}s`;
    } else {
      return `${seconds}s`;
    }
  } catch (error) {
    console.warn('Duration formatting error:', error);
    return '-';
  }
};

/**
 * Check if a date is in the recent past (within specified hours)
 * Useful for determining bot status freshness
 * 
 * @param dateInput - Date string or Date object to check
 * @param withinHours - Number of hours to consider as "recent" (default: 1)
 * @returns True if date is within the specified timeframe
 */
export const isRecent = (
  dateInput: string | Date,
  withinHours: number = 1
): boolean => {
  try {
    const date = parseDate(dateInput);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffHours = diffMs / 3600000;
    
    return diffHours <= withinHours && diffHours >= 0;
  } catch (error) {
    return false;
  }
};