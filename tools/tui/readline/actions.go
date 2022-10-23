// License: GPLv3 Copyright: 2022, Kovid Goyal, <kovid at kovidgoyal.net>

package readline

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	"kitty/tools/utils"
	"kitty/tools/wcswidth"
)

var _ = fmt.Print

func (self *Readline) text_upto_cursor_pos() string {
	buf := strings.Builder{}
	buf.Grow(1024)
	for i, line := range self.lines {
		if i == self.cursor.Y {
			buf.WriteString(line[:utils.Min(len(line), self.cursor.X)])
			break
		} else {
			buf.WriteString(line)
			buf.WriteString("\n")
		}
	}
	return buf.String()
}

func (self *Readline) text_after_cursor_pos() string {
	buf := strings.Builder{}
	buf.Grow(1024)
	for i, line := range self.lines {
		if i == self.cursor.Y {
			buf.WriteString(line[utils.Min(len(line), self.cursor.X):])
			buf.WriteString("\n")
		} else if i > self.cursor.Y {
			buf.WriteString(line)
			buf.WriteString("\n")
		}
	}
	ans := buf.String()
	ans = ans[:len(ans)-1]
	return ans
}

func (self *Readline) all_text() string {
	return strings.Join(self.lines, "\n")
}

func (self *Readline) add_text(text string) {
	new_lines := make([]string, 0, len(self.lines)+4)
	new_lines = append(new_lines, self.lines[:self.cursor.Y]...)
	var lines_after []string
	if len(self.lines) > self.cursor.Y+1 {
		lines_after = self.lines[self.cursor.Y+1:]
	}
	has_trailing_newline := strings.HasSuffix(text, "\n")

	add_line_break := func(line string) {
		new_lines = append(new_lines, line)
		self.cursor.X = len(line)
		self.cursor.Y += 1
	}
	cline := self.lines[self.cursor.Y]
	before_first_line := cline[:self.cursor.X]
	after_first_line := ""
	if self.cursor.X < len(cline) {
		after_first_line = cline[self.cursor.X:]
	}
	for i, line := range utils.Splitlines(text) {
		if i > 0 {
			add_line_break(line)
		} else {
			line := before_first_line + line
			self.cursor.X = len(line)
			new_lines = append(new_lines, line)
		}
	}
	if has_trailing_newline {
		add_line_break("")
	}
	if after_first_line != "" {
		if len(new_lines) == 0 {
			new_lines = append(new_lines, "")
		}
		new_lines[len(new_lines)-1] += after_first_line
	}
	if len(lines_after) > 0 {
		new_lines = append(new_lines, lines_after...)
	}
	self.lines = new_lines
}

func (self *Readline) move_cursor_left(amt uint, traverse_line_breaks bool) (amt_moved uint) {
	for amt_moved < amt {
		if self.cursor.X == 0 {
			if !traverse_line_breaks || self.cursor.Y == 0 {
				return amt_moved
			}
			self.cursor.Y -= 1
			self.cursor.X = len(self.lines[self.cursor.Y])
			amt_moved++
			continue
		}
		line := self.lines[self.cursor.Y]
		for ci := wcswidth.NewCellIterator(line[:self.cursor.X]).GotoEnd(); amt_moved < amt && ci.Backward(); amt_moved++ {
			self.cursor.X -= len(ci.Current())
		}
	}
	return amt_moved
}

func (self *Readline) move_cursor_right(amt uint, traverse_line_breaks bool) (amt_moved uint) {
	for amt_moved < amt {
		line := self.lines[self.cursor.Y]
		if self.cursor.X >= len(line) {
			if !traverse_line_breaks || self.cursor.Y == len(self.lines)-1 {
				return amt_moved
			}
			self.cursor.Y += 1
			self.cursor.X = 0
			amt_moved++
			continue
		}

		for ci := wcswidth.NewCellIterator(line[self.cursor.X:]); amt_moved < amt && ci.Forward(); amt_moved++ {
			self.cursor.X += len(ci.Current())
		}
	}
	return amt_moved
}

func (self *Readline) move_cursor_to_target_line(source_line, target_line *ScreenLine) {
	if source_line != target_line {
		visual_distance_into_text := source_line.CursorCell - source_line.PromptLen
		self.cursor.Y = target_line.ParentLineNumber
		tp := wcswidth.TruncateToVisualLength(target_line.Text, visual_distance_into_text)
		self.cursor.X = target_line.OffsetInParentLine + len(tp)
	}
}

func (self *Readline) move_cursor_vertically(amt int) (ans int) {
	if self.screen_width == 0 {
		self.update_current_screen_size()
	}
	screen_lines := self.get_screen_lines()
	cursor_line_num := 0
	for i, sl := range screen_lines {
		if sl.CursorCell > -1 {
			cursor_line_num = i
			break
		}
	}
	target_line_num := utils.Min(utils.Max(0, cursor_line_num+amt), len(screen_lines)-1)
	ans = target_line_num - cursor_line_num
	if ans != 0 {
		self.move_cursor_to_target_line(screen_lines[cursor_line_num], screen_lines[target_line_num])
	}
	return ans
}

func (self *Readline) move_cursor_down(amt uint) uint {
	ans := uint(0)
	if self.screen_width == 0 {
		self.update_current_screen_size()
	}
	return ans
}

func (self *Readline) move_to_start_of_line() bool {
	if self.cursor.X > 0 {
		self.cursor.X = 0
		return true
	}
	return false
}

func (self *Readline) move_to_end_of_line() bool {
	line := self.lines[self.cursor.Y]
	if self.cursor.X >= len(line) {
		return false
	}
	self.cursor.X = len(line)
	return true
}

func (self *Readline) move_to_start() bool {
	if self.cursor.Y == 0 && self.cursor.X == 0 {
		return false
	}
	self.cursor.Y = 0
	self.move_to_start_of_line()
	return true
}

func (self *Readline) move_to_end() bool {
	line := self.lines[self.cursor.Y]
	if self.cursor.Y == len(self.lines)-1 && self.cursor.X >= len(line) {
		return false
	}
	self.cursor.Y = len(self.lines) - 1
	self.move_to_end_of_line()
	return true
}

func (self *Readline) erase_between(start, end Position) {
	if end.Less(start) {
		start, end = end, start
	}
	if start.Y == end.Y {
		line := self.lines[start.Y]
		self.lines[start.Y] = line[:start.X] + line[end.X:]
		if self.cursor.Y == start.Y && self.cursor.X >= start.X {
			if self.cursor.X < end.X {
				self.cursor.X = start.X
			} else {
				self.cursor.X -= end.X - start.X
			}
		}
		return
	}
	lines := make([]string, 0, len(self.lines))
	for i, line := range self.lines {
		if i < start.Y || i > end.Y {
			lines = append(lines, line)
		} else if i == start.Y {
			lines = append(lines, line[:start.X])
			if self.cursor.Y == i && self.cursor.X > start.X {
				self.cursor.X = start.X
			}
		} else if i == end.Y {
			lines[len(lines)-1] += line[end.X:]
			if i == self.cursor.Y {
				self.cursor.Y = start.Y
				if self.cursor.X < end.X {
					self.cursor.X = start.X
				} else {
					self.cursor.X -= end.X - start.X
				}
			}
		} else if i == self.cursor.Y {
			self.cursor = start
		}
	}
	self.lines = lines
}

func (self *Readline) erase_chars_before_cursor(amt uint, traverse_line_breaks bool) uint {
	pos := self.cursor
	num := self.move_cursor_left(amt, traverse_line_breaks)
	if num == 0 {
		return num
	}
	self.erase_between(self.cursor, pos)
	return num
}

func (self *Readline) erase_chars_after_cursor(amt uint, traverse_line_breaks bool) uint {
	pos := self.cursor
	num := self.move_cursor_right(amt, traverse_line_breaks)
	if num == 0 {
		return num
	}
	self.erase_between(pos, self.cursor)
	return num
}

func has_word_chars(text string) bool {
	for _, ch := range text {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) {
			return true
		}
	}
	return false
}

func (self *Readline) move_to_end_of_word(amt uint, traverse_line_breaks bool) (num_of_words_moved uint) {
	if amt == 0 {
		return 0
	}
	line := self.lines[self.cursor.Y]
	in_word := false
	ci := wcswidth.NewCellIterator(line[self.cursor.X:])
	sz := 0

	for ci.Forward() {
		current_is_word_char := has_word_chars(ci.Current())
		plen := sz
		sz += len(ci.Current())
		if current_is_word_char {
			in_word = true
		} else if in_word {
			self.cursor.X += plen
			amt--
			num_of_words_moved++
			if amt == 0 {
				return
			}
			in_word = false
		}
	}
	if self.move_to_end_of_line() {
		amt--
		num_of_words_moved++
	}
	if amt > 0 {
		if traverse_line_breaks && self.cursor.Y < len(self.lines)-1 {
			self.cursor.Y++
			self.cursor.X = 0
			num_of_words_moved += self.move_to_end_of_word(amt, traverse_line_breaks)
		}
	}
	return
}

func (self *Readline) move_to_start_of_word(amt uint, traverse_line_breaks bool) (num_of_words_moved uint) {
	if amt == 0 {
		return 0
	}
	line := self.lines[self.cursor.Y]
	in_word := false
	ci := wcswidth.NewCellIterator(line[:self.cursor.X]).GotoEnd()
	sz := 0

	for ci.Backward() {
		current_is_word_char := has_word_chars(ci.Current())
		plen := sz
		sz += len(ci.Current())
		if current_is_word_char {
			in_word = true
		} else if in_word {
			self.cursor.X -= plen
			amt--
			num_of_words_moved++
			if amt == 0 {
				return
			}
			in_word = false
		}
	}
	if self.move_to_start_of_line() {
		amt--
		num_of_words_moved++
	}
	if amt > 0 {
		if traverse_line_breaks && self.cursor.Y > 0 {
			self.cursor.Y--
			self.cursor.X = len(self.lines[self.cursor.Y])
			num_of_words_moved += self.move_to_start_of_word(amt, traverse_line_breaks)
		}
	}
	return
}

func (self *Readline) perform_action(ac Action, repeat_count uint) error {
	switch ac {
	case ActionBackspace:
		if self.erase_chars_before_cursor(repeat_count, true) > 0 {
			return nil
		}
	case ActionDelete:
		if self.erase_chars_after_cursor(repeat_count, true) > 0 {
			return nil
		}
	case ActionMoveToStartOfLine:
		if self.move_to_start_of_line() {
			return nil
		}
	case ActionMoveToEndOfLine:
		if self.move_to_end_of_line() {
			return nil
		}
	case ActionMoveToEndOfWord:
		if self.move_to_end_of_word(repeat_count, true) > 0 {
			return nil
		}
	case ActionMoveToStartOfWord:
		if self.move_to_start_of_word(repeat_count, true) > 0 {
			return nil
		}
	case ActionMoveToStartOfDocument:
		if self.move_to_start() {
			return nil
		}
	case ActionMoveToEndOfDocument:
		if self.move_to_end() {
			return nil
		}
	case ActionCursorLeft:
		if self.move_cursor_left(repeat_count, true) > 0 {
			return nil
		}
	case ActionCursorRight:
		if self.move_cursor_right(repeat_count, true) > 0 {
			return nil
		}
	case ActionEndInput:
		line := self.lines[self.cursor.Y]
		if line == "" {
			return io.EOF
		}
		return self.perform_action(ActionAcceptInput, 1)
	case ActionAcceptInput:
		return ErrAcceptInput
	case ActionCursorUp:
		if self.move_cursor_vertically(-int(repeat_count)) != 0 {
			return nil
		}
	case ActionCursorDown:
		if self.move_cursor_vertically(int(repeat_count)) != 0 {
			return nil
		}
	case ActionHistoryPreviousOrCursorUp:
		if self.cursor.Y == 0 {
			r := self.perform_action(ActionHistoryPrevious, repeat_count)
			if r == nil {
				return nil
			}
		}
		return self.perform_action(ActionCursorUp, repeat_count)
	case ActionHistoryNextOrCursorDown:
		if self.cursor.Y == 0 {
			r := self.perform_action(ActionHistoryNext, repeat_count)
			if r == nil {
				return nil
			}
		}
		return self.perform_action(ActionCursorDown, repeat_count)

	}
	return ErrCouldNotPerformAction
}