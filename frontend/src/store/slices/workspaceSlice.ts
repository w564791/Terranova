import { createSlice, type PayloadAction } from '@reduxjs/toolkit';

interface Workspace {
  id: number;
  name: string;
  description: string;
  state_backend: string;
  terraform_version: string;
  execution_mode: string;
  created_at: string;
}

interface WorkspaceState {
  workspaces: Workspace[];
  loading: boolean;
  total: number;
  page: number;
  size: number;
}

const initialState: WorkspaceState = {
  workspaces: [],
  loading: false,
  total: 0,
  page: 1,
  size: 20,
};

const workspaceSlice = createSlice({
  name: 'workspaces',
  initialState,
  reducers: {
    setLoading: (state, action: PayloadAction<boolean>) => {
      state.loading = action.payload;
    },
    setWorkspaces: (state, action: PayloadAction<{ items: Workspace[]; total: number; page: number; size: number }>) => {
      state.workspaces = action.payload.items;
      state.total = action.payload.total;
      state.page = action.payload.page;
      state.size = action.payload.size;
    },
    addWorkspace: (state, action: PayloadAction<Workspace>) => {
      state.workspaces.unshift(action.payload);
    },
    updateWorkspace: (state, action: PayloadAction<Workspace>) => {
      const index = state.workspaces.findIndex(w => w.id === action.payload.id);
      if (index !== -1) {
        state.workspaces[index] = action.payload;
      }
    },
    removeWorkspace: (state, action: PayloadAction<number>) => {
      state.workspaces = state.workspaces.filter(w => w.id !== action.payload);
    },
  },
});

export const { setLoading, setWorkspaces, addWorkspace, updateWorkspace, removeWorkspace } = workspaceSlice.actions;
export default workspaceSlice.reducer;