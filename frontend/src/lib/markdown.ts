import { marked } from 'marked'
import DOMPurify from 'dompurify'

marked.setOptions({ breaks: true })

// Issue descriptions and comments come from external servers (Redmine,
// GitHub, GitLab) that may format with Markdown — render it, but sanitize
// since this is `{@html}`-inserted content we don't control.
export function renderMarkdown(text: string): string {
  if (!text) return ''
  const html = marked.parse(text, { async: false }) as string
  return DOMPurify.sanitize(html, { ADD_ATTR: ['target', 'rel'] })
}
