// Typed wrappers around the Wails-bound Go App methods.
import type {
  ChapterInsight,
  ChapterPlan,
  CodexEntry,
  CreateChapterResult,
  ScanResult,
  Schema,
  SeriesPlan,
  Settings,
  Suggestion,
  Workspace,
} from './types'

/* eslint-disable @typescript-eslint/no-explicit-any */
declare global {
  interface Window {
    go: { main: { App: Record<string, (...args: any[]) => Promise<any>> } }
  }
}

const app = () => window.go.main.App

export const SelectFolder = (title: string): Promise<string> => app().SelectFolder(title)

export const CreateWorkspace = (path: string, name: string, kind: string): Promise<Workspace> =>
  app().CreateWorkspace(path, name, kind)

export const OpenWorkspace = (path: string): Promise<Workspace> => app().OpenWorkspace(path)

export const SaveCodexEntry = (
  entry: CodexEntry,
  oldType: string,
  oldScope: string,
): Promise<Workspace> => app().SaveCodexEntry(entry, oldType, oldScope)

export const DeleteCodexEntry = (entry: CodexEntry): Promise<Workspace> =>
  app().DeleteCodexEntry(entry)

export const ReadChapter = (bookId: string, chapter: string): Promise<string> =>
  app().ReadChapter(bookId, chapter)

export const SaveChapter = (bookId: string, chapter: string, content: string): Promise<void> =>
  app().SaveChapter(bookId, chapter, content)

export const CreateChapter = (bookId: string, title: string): Promise<CreateChapterResult> =>
  app().CreateChapter(bookId, title)

export const CreateBook = (title: string): Promise<Workspace> => app().CreateBook(title)

export const SaveSchema = (schema: Schema): Promise<Workspace> => app().SaveSchema(schema)

export const ScanText = (bookId: string, chapter: string, text: string): Promise<ScanResult> =>
  app().ScanText(bookId, chapter, text)

export const GetSettings = (): Promise<Settings> => app().GetSettings()

export const SaveSettings = (s: Settings): Promise<Settings> => app().SaveSettings(s)

export const CloseWorkspace = (): Promise<void> => app().CloseWorkspace()

export const DeepScan = (bookId: string, chapter: string, text: string): Promise<Suggestion[]> =>
  app().DeepScan(bookId, chapter, text)

export const SpellSuggest = (word: string): Promise<string[]> => app().SpellSuggest(word)

export const AddToDictionary = (word: string): Promise<void> => app().AddToDictionary(word)

export const SpellStatus = (): Promise<string> => app().SpellStatus()

export const SavePlan = (bookId: string, plan: ChapterPlan[]): Promise<Workspace> =>
  app().SavePlan(bookId, plan)

export const MoveChapter = (bookId: string, chapter: string, delta: number): Promise<Workspace> =>
  app().MoveChapter(bookId, chapter, delta)

export const BookInsights = (bookId: string): Promise<Record<string, ChapterInsight>> =>
  app().BookInsights(bookId)

export const SaveSeriesPlan = (plan: SeriesPlan): Promise<Workspace> => app().SaveSeriesPlan(plan)

export const MoveBook = (bookId: string, delta: number): Promise<Workspace> =>
  app().MoveBook(bookId, delta)
