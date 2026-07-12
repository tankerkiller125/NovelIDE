// Typed wrappers around the Wails-bound Go App methods.
import type {
  Backlink,
  ChapterInsight,
  DiffResult,
  FileChange,
  Snapshot,
  ChapterPlan,
  ChapterScenes,
  CodexEntry,
  CreateChapterResult,
  ExportOptions,
  ExportTheme,
  RenameChapterResult,
  AuthConfig,
  ScanResult,
  SearchResults,
  SyncOutcome,
  SyncStatus,
  RemoteWorkspace,
  WritingStats,
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

export const AppVersion = (): Promise<string> => app().AppVersion()

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

export const PickEntryImage = (entry: CodexEntry): Promise<Workspace> =>
  app().PickEntryImage(entry)

export const ClearEntryImage = (entry: CodexEntry): Promise<Workspace> =>
  app().ClearEntryImage(entry)

export const ReadImageDataURL = (rel: string): Promise<string> => app().ReadImageDataURL(rel)

export const ReadChapter = (bookId: string, chapter: string): Promise<string> =>
  app().ReadChapter(bookId, chapter)

export const SaveChapter = (bookId: string, chapter: string, content: string): Promise<void> =>
  app().SaveChapter(bookId, chapter, content)

export const CreateChapter = (bookId: string, title: string): Promise<CreateChapterResult> =>
  app().CreateChapter(bookId, title)

export const CreateBook = (title: string): Promise<Workspace> => app().CreateBook(title)

export const RenameChapter = (
  bookId: string,
  chapter: string,
  newTitle: string,
): Promise<RenameChapterResult> => app().RenameChapter(bookId, chapter, newTitle)

export const DeleteChapter = (bookId: string, chapter: string): Promise<Workspace> =>
  app().DeleteChapter(bookId, chapter)

export const RenameBook = (bookId: string, newTitle: string): Promise<Workspace> =>
  app().RenameBook(bookId, newTitle)

export const DeleteBook = (bookId: string): Promise<Workspace> => app().DeleteBook(bookId)

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

export const ExportThemes = (): Promise<ExportTheme[]> => app().ExportThemes()

export const ExportPreview = (opts: ExportOptions): Promise<string> => app().ExportPreview(opts)

export const ExportSave = (opts: ExportOptions): Promise<string> => app().ExportSave(opts)

export const RecordWritingProgress = (): Promise<WritingStats> => app().RecordWritingProgress()

export const SetDailyGoal = (goal: number): Promise<WritingStats> => app().SetDailyGoal(goal)

// --- optional sync ---
export const SyncStatusGet = (): Promise<SyncStatus> => app().SyncStatus()

export const SyncRegister = (
  server: string,
  username: string,
  password: string,
): Promise<SyncStatus> => app().SyncRegister(server, username, password)

export const SyncLogin = (
  server: string,
  username: string,
  password: string,
): Promise<SyncStatus> => app().SyncLogin(server, username, password)

export const SyncLogout = (): Promise<SyncStatus> => app().SyncLogout()

export const SyncAuthConfig = (server: string): Promise<AuthConfig> =>
  app().SyncAuthConfig(server)

export const SyncLoginSSO = (server: string): Promise<SyncStatus> => app().SyncLoginSSO(server)

export const RemoteWorkspaces = (): Promise<RemoteWorkspace[]> => app().RemoteWorkspaces()

export const SyncNow = (): Promise<SyncOutcome> => app().SyncNow()

export const SyncLinkPull = (remoteId: string): Promise<SyncOutcome> =>
  app().SyncLinkPull(remoteId)

export const Backlinks = (entryId: string): Promise<Backlink[]> => app().Backlinks(entryId)

export const CreateSnapshot = (label: string): Promise<Snapshot[]> => app().CreateSnapshot(label)

export const ListSnapshots = (): Promise<Snapshot[]> => app().ListSnapshots()

export const SnapshotChanges = (id: string): Promise<FileChange[]> => app().SnapshotChanges(id)

export const SnapshotFileDiff = (id: string, rel: string): Promise<DiffResult> =>
  app().SnapshotFileDiff(id, rel)

export const RestoreSnapshotFile = (id: string, rel: string): Promise<Workspace> =>
  app().RestoreSnapshotFile(id, rel)

export const RestoreSnapshot = (id: string): Promise<Workspace> => app().RestoreSnapshot(id)

export const DeleteSnapshot = (id: string): Promise<Snapshot[]> => app().DeleteSnapshot(id)

export const SearchProject = (
  query: string,
  caseSensitive: boolean,
  wholeWord: boolean,
): Promise<SearchResults> => app().SearchProject(query, caseSensitive, wholeWord)

export const ReplaceAllProject = (
  query: string,
  replacement: string,
  caseSensitive: boolean,
  wholeWord: boolean,
): Promise<number> => app().ReplaceAllProject(query, replacement, caseSensitive, wholeWord)

export const BookScenes = (bookId: string): Promise<ChapterScenes[]> => app().BookScenes(bookId)

export const MoveScene = (
  bookId: string,
  srcChapter: string,
  sceneIndex: number,
  dstChapter: string,
  dstIndex: number,
): Promise<ChapterScenes[]> =>
  app().MoveScene(bookId, srcChapter, sceneIndex, dstChapter, dstIndex)

export const SetSceneTitle = (
  bookId: string,
  chapter: string,
  sceneIndex: number,
  title: string,
): Promise<ChapterScenes[]> => app().SetSceneTitle(bookId, chapter, sceneIndex, title)
