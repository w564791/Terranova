import { createSlice, type PayloadAction } from '@reduxjs/toolkit';

interface Module {
  id: number;
  name: string;
  provider: string;
  source: string;
  version: string;
  description: string;
  created_at: string;
}

interface ModuleState {
  modules: Module[];
  loading: boolean;
  total: number;
  page: number;
  size: number;
}

const initialState: ModuleState = {
  modules: [],
  loading: false,
  total: 0,
  page: 1,
  size: 20,
};

const moduleSlice = createSlice({
  name: 'modules',
  initialState,
  reducers: {
    setLoading: (state, action: PayloadAction<boolean>) => {
      state.loading = action.payload;
    },
    setModules: (state, action: PayloadAction<{ items: Module[]; total: number; page: number; size: number }>) => {
      state.modules = action.payload.items;
      state.total = action.payload.total;
      state.page = action.payload.page;
      state.size = action.payload.size;
    },
    addModule: (state, action: PayloadAction<Module>) => {
      state.modules.unshift(action.payload);
    },
    updateModule: (state, action: PayloadAction<Module>) => {
      const index = state.modules.findIndex(m => m.id === action.payload.id);
      if (index !== -1) {
        state.modules[index] = action.payload;
      }
    },
    removeModule: (state, action: PayloadAction<number>) => {
      state.modules = state.modules.filter(m => m.id !== action.payload);
    },
  },
});

export const { setLoading, setModules, addModule, updateModule, removeModule } = moduleSlice.actions;
export default moduleSlice.reducer;