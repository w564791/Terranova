import React, { useState, useRef } from 'react';
import DatePicker from 'react-datepicker';
import 'react-datepicker/dist/react-datepicker.css';
import styles from './DateRangePicker.module.css';

interface DateRangePickerProps {
  startDate: string;
  endDate: string;
  onApply: (startDate: string, endDate: string) => void;
  onClear: () => void;
  onCancel: () => void;
}

const DateRangePicker: React.FC<DateRangePickerProps> = ({
  startDate,
  endDate,
  onApply,
  onClear,
  onCancel,
}) => {
  const [tempStartDate, setTempStartDate] = useState<Date | null>(
    startDate ? new Date(startDate) : null
  );
  const [tempEndDate, setTempEndDate] = useState<Date | null>(
    endDate ? new Date(endDate) : null
  );
  const [isOpen, setIsOpen] = useState(false);
  const pickerRef = useRef<HTMLDivElement>(null);

  // 处理日历日期双击 - 快速选择整天
  const handleDayDoubleClick = (date: Date) => {
    // 设置开始时间为当天00:00:00
    const start = new Date(date);
    start.setHours(0, 0, 0, 0);
    
    // 设置结束时间为当天23:59:59
    const end = new Date(date);
    end.setHours(23, 59, 59, 999);
    
    setTempStartDate(start);
    setTempEndDate(end);
  };

  // 处理Apply按钮
  const handleApply = () => {
    if (tempStartDate && tempEndDate) {
      // 确保开始时间是00:00:00
      const start = new Date(tempStartDate);
      start.setHours(0, 0, 0, 0);
      
      // 确保结束时间是23:59:59
      const end = new Date(tempEndDate);
      end.setHours(23, 59, 59, 999);
      
      onApply(start.toISOString().split('T')[0], end.toISOString().split('T')[0]);
      setIsOpen(false);
    }
  };

  // 处理Clear按钮
  const handleClear = () => {
    setTempStartDate(null);
    setTempEndDate(null);
    onClear();
    setIsOpen(false);
  };

  // 处理Cancel按钮
  const handleCancel = () => {
    // 恢复原始值
    setTempStartDate(startDate ? new Date(startDate) : null);
    setTempEndDate(endDate ? new Date(endDate) : null);
    onCancel();
    setIsOpen(false);
  };

  // 格式化显示日期
  const formatDisplayDate = (date: Date | null) => {
    if (!date) return 'Select date';
    return date.toLocaleDateString('en-GB', {
      day: '2-digit',
      month: '2-digit',
      year: 'numeric',
    });
  };

  return (
    <div className={styles.container} ref={pickerRef}>
      {/* 日期输入框 */}
      <div className={styles.inputRow}>
        <div className={styles.inputGroup}>
          <label className={styles.label}>Start Date</label>
          <button
            className={styles.dateButton}
            onClick={() => setIsOpen(!isOpen)}
          >
            {formatDisplayDate(tempStartDate)}
          </button>
        </div>
        
        <span className={styles.arrow}>→</span>
        
        <div className={styles.inputGroup}>
          <label className={styles.label}>End Date</label>
          <button
            className={styles.dateButton}
            onClick={() => setIsOpen(!isOpen)}
          >
            {formatDisplayDate(tempEndDate)}
          </button>
        </div>
      </div>

      {/* 日历弹出面板 */}
      {isOpen && (
        <div className={styles.pickerPanel}>
          {/* 双月日历视图 */}
          <div className={styles.calendarContainer}>
            <DatePicker
              selected={tempStartDate}
              onChange={(dates) => {
                const [start, end] = dates as [Date | null, Date | null];
                setTempStartDate(start);
                setTempEndDate(end);
              }}
              startDate={tempStartDate}
              endDate={tempEndDate}
              selectsRange
              inline
              monthsShown={2}
              calendarClassName={styles.calendar}
              onSelect={(date) => {
                if (date) {
                  // 检测是否是双击
                  const now = Date.now();
                  const lastClick = (window as any).lastDateClick || 0;
                  if (now - lastClick < 300) {
                    // 双击 - 快速选择整天
                    handleDayDoubleClick(date);
                  }
                  (window as any).lastDateClick = now;
                }
              }}
            />
          </div>

          {/* 操作按钮 */}
          <div className={styles.actions}>
            <button className={styles.clearButton} onClick={handleClear}>
              Clear
            </button>
            <div className={styles.actionRight}>
              <button className={styles.cancelButton} onClick={handleCancel}>
                Cancel
              </button>
              <button
                className={styles.applyButton}
                onClick={handleApply}
                disabled={!tempStartDate || !tempEndDate}
              >
                Apply
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default DateRangePicker;
