#!/usr/bin/env python3
"""
Script to replace log.Printf/Print/Println with logger.Debug
This script provides more intelligent replacement than the bash version.

Usage: python3 replace_log_with_logger.py [directory]
"""

import os
import re
import sys
from pathlib import Path
from typing import List, Tuple

def should_skip_file(filepath: str) -> bool:
    """Check if file should be skipped"""
    if '_test.go' in filepath:
        return True
    if '/vendor/' in filepath:
        return True
    if '/.git/' in filepath:
        return True
    return False

def has_log_calls(content: str) -> bool:
    """Check if content has log.Printf/Print/Println calls"""
    patterns = [
        r'log\.Printf\(',
        r'log\.Print\(',
        r'log\.Println\(',
    ]
    for pattern in patterns:
        if re.search(pattern, content):
            return True
    return False

def replace_log_printf(content: str) -> Tuple[str, int]:
    """Replace log.Printf with logger.Debug"""
    pattern = r'log\.Printf\('
    replacement = r'logger.Debug('
    new_content, count = re.subn(pattern, replacement, content)
    return new_content, count

def replace_log_print(content: str) -> Tuple[str, int]:
    """Replace log.Print with logger.Debug
    
    This is more complex because log.Print doesn't use format strings.
    We need to convert:
        log.Print("message")  ->  logger.Debug("%s", "message")
        log.Print(variable)   ->  logger.Debug("%v", variable)
    """
    # Pattern to match log.Print with single string argument
    pattern1 = r'log\.Print\("([^"]+)"\)'
    replacement1 = r'logger.Debug("\1")'
    new_content, count1 = re.subn(pattern1, replacement1, content)
    
    # Pattern to match log.Print with variable or expression
    pattern2 = r'log\.Print\(([^)]+)\)'
    replacement2 = r'logger.Debug("%v", \1)'
    new_content, count2 = re.subn(pattern2, replacement2, new_content)
    
    return new_content, count1 + count2

def replace_log_println(content: str) -> Tuple[str, int]:
    """Replace log.Println with logger.Debug
    
    Similar to log.Print but adds newline (which logger.Debug does automatically)
    """
    # Pattern to match log.Println with single string argument
    pattern1 = r'log\.Println\("([^"]+)"\)'
    replacement1 = r'logger.Debug("\1")'
    new_content, count1 = re.subn(pattern1, replacement1, content)
    
    # Pattern to match log.Println with variable or expression
    pattern2 = r'log\.Println\(([^)]+)\)'
    replacement2 = r'logger.Debug("%v", \1)'
    new_content, count2 = re.subn(pattern2, replacement2, new_content)
    
    return new_content, count1 + count2

def needs_logger_param(content: str, filepath: str) -> bool:
    """Check if file needs logger parameter added to functions"""
    # Skip if it's already using TerraformLogger
    if 'logger *TerraformLogger' in content or 'logger *services.TerraformLogger' in content:
        return False
    
    # Skip if it's in services package (likely already has logger)
    if '/services/' in filepath:
        return False
    
    # Check if logger.Debug is used but no logger parameter
    if 'logger.Debug' in content and 'logger *' not in content:
        return True
    
    return False

def process_file(filepath: str, dry_run: bool = False) -> Tuple[int, bool]:
    """Process a single Go file
    
    Returns: (number of replacements, needs_logger_param)
    """
    with open(filepath, 'r', encoding='utf-8') as f:
        original_content = f.read()
    
    if not has_log_calls(original_content):
        return 0, False
    
    content = original_content
    total_replacements = 0
    
    # Replace log.Printf
    content, count = replace_log_printf(content)
    if count > 0:
        print(f"  - Replaced {count} log.Printf calls")
        total_replacements += count
    
    # Replace log.Print
    content, count = replace_log_print(content)
    if count > 0:
        print(f"  - Replaced {count} log.Print calls")
        total_replacements += count
    
    # Replace log.Println
    content, count = replace_log_println(content)
    if count > 0:
        print(f"  - Replaced {count} log.Println calls")
        total_replacements += count
    
    needs_param = needs_logger_param(content, filepath)
    
    if total_replacements > 0 and not dry_run:
        # Create backup
        backup_path = filepath + '.bak'
        with open(backup_path, 'w', encoding='utf-8') as f:
            f.write(original_content)
        
        # Write modified content
        with open(filepath, 'w', encoding='utf-8') as f:
            f.write(content)
    
    return total_replacements, needs_param

def main():
    target_dir = sys.argv[1] if len(sys.argv) > 1 else 'backend'
    dry_run = '--dry-run' in sys.argv
    
    if not os.path.isdir(target_dir):
        print(f"Error: Directory {target_dir} does not exist")
        sys.exit(1)
    
    print(f"Starting log replacement in {target_dir}...")
    if dry_run:
        print("DRY RUN MODE - No files will be modified")
    print("This will replace log.Printf/Print/Println with logger.Debug")
    print()
    
    total_files = 0
    modified_files = 0
    total_replacements = 0
    files_needing_logger = []
    
    # Find all .go files
    for filepath in Path(target_dir).rglob('*.go'):
        filepath_str = str(filepath)
        total_files += 1
        
        if should_skip_file(filepath_str):
            continue
        
        try:
            replacements, needs_param = process_file(filepath_str, dry_run)
            
            if replacements > 0:
                print(f"Processing: {filepath_str}")
                modified_files += 1
                total_replacements += replacements
                print(f"  ✓ Total replacements in file: {replacements}")
                
                if needs_param:
                    files_needing_logger.append(filepath_str)
                    print(f"    Warning: This file may need logger parameter added to functions")
                
                print()
        except Exception as e:
            print(f"Error processing {filepath_str}: {e}")
    
    print("=" * 50)
    print("Replacement Summary:")
    print("=" * 50)
    print(f"Total files scanned: {total_files}")
    print(f"Files modified: {modified_files}")
    print(f"Total replacements: {total_replacements}")
    
    if files_needing_logger:
        print(f"\n  Files that may need logger parameter ({len(files_needing_logger)}):")
        for f in files_needing_logger:
            print(f"  - {f}")
    
    if not dry_run and modified_files > 0:
        print("\nBackup files created with .bak extension")
    
    print("\n  IMPORTANT NOTES:")
    print("1. Review the changes carefully before committing")
    print("2. Some files may need logger parameter added to functions")
    print("3. Test the application thoroughly after changes")
    
    if not dry_run:
        print(f"4. You can restore backups with: find {target_dir} -name '*.bak' -exec sh -c 'mv \"$1\" \"${{1%.bak}}\"' _ {{}} \\;")
        print(f"\nTo remove backup files after verification:")
        print(f"  find {target_dir} -name '*.bak' -delete")
    else:
        print("\nRun without --dry-run to apply changes")

if __name__ == '__main__':
    main()
