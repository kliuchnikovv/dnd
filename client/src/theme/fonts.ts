import { useFonts } from 'expo-font';
import {
    PTSerif_400Regular,
    PTSerif_400Regular_Italic,
    PTSerif_700Bold,
    PTSerif_700Bold_Italic,
} from '@expo-google-fonts/pt-serif';
import {
    IBMPlexMono_400Regular,
    IBMPlexMono_500Medium,
    IBMPlexMono_700Bold,
} from '@expo-google-fonts/ibm-plex-mono';
import {
    PTSans_400Regular,
    PTSans_700Bold,
} from '@expo-google-fonts/pt-sans';

// The three voices of «Ночной детектив». Family keys here must equal the fontFamily
// strings used in theme/nightDetective.ts.
export function useAppFonts(): boolean {
    const [loaded] = useFonts({
        PTSerif_400Regular,
        PTSerif_400Regular_Italic,
        PTSerif_700Bold,
        PTSerif_700Bold_Italic,
        IBMPlexMono_400Regular,
        IBMPlexMono_500Medium,
        IBMPlexMono_700Bold,
        PTSans_400Regular,
        PTSans_700Bold,
    });
    return loaded;
}
