import { LoginForm } from "../auth";
import { Container } from "@mui/material";

export interface LoginProps {
    onLogin: (email: string, password: string, rememberMe: boolean) => void;
}

export function LoginPage(props: LoginProps) {
  return (
      <Container>
        <LoginForm onLogin={props.onLogin}/>
      </Container>
  );
}
