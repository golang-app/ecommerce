import { AppNavBar, Footer } from "../tetris";
import { LoginForm } from "../auth";
import { Container } from "@mui/material";

export interface LoginProps {
    onLogin: (email: string, password: string, rememberMe: boolean) => void;
    loggedIn: boolean;
}

export function LoginPage(props: LoginProps) {
  return (
    <div>
      <AppNavBar loggedIn={false} cartItems={[]} />
      <Container>
        <LoginForm onLogin={props.onLogin}/>
      </Container>
      <Footer />
    </div>
  );
}
